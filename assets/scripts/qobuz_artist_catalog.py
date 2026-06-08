#!/usr/bin/env python3
import json
import re
import sys

sys.dont_write_bytecode = True

from qobuz_common import load_client, run_with_qobuz_error_handling

JSON_MARKER = "__LOADER21_QOBUZ_JSON__"


def classify_release(album):
    product_type = str(album.get("product_type") or "").strip().lower()
    release_type = str(album.get("release_type") or "").strip().lower()
    title = str(album.get("title") or "").strip().lower()
    version = str(album.get("version") or "").strip().lower()
    genre_name = str((album.get("genre") or {}).get("name") or "").strip().lower()
    genres_list = " ".join(str(value or "").strip().lower() for value in (album.get("genres_list") or []))
    text = f"{title} {version} {product_type} {release_type} {genre_name} {genres_list}"

    if product_type == "audiobook" or release_type == "audiobook":
        return "Audiobook"
    if re.search(r"\b(audiobook|audio\s*book|livre\s+audio|spoken\s+word|unabridged|abridged)\b", text):
        return "Audiobook"

    if re.search(r"\blive\b", text):
        return "Live"
    if re.search(r"\b(single|single-track)\b", text):
        return "Single"
    if re.search(r"\bep\b|\be\.p\.\b", text):
        return "EP"
    if re.search(r"\b(compilation|best of|anthology|collection)\b", text):
        return "Compilation"
    if re.search(r"\b(ost|soundtrack|bande originale|original motion picture)\b", text):
        return "OST"
    if re.search(r"\b(album|lp)\b", text):
        return "Album"

    tracks_count = album.get("tracks_count")
    if isinstance(tracks_count, int):
        duration_value = album.get("duration")
        duration_seconds = None
        if isinstance(duration_value, (int, float)):
            duration_seconds = int(duration_value)
        elif isinstance(duration_value, str):
            try:
                duration_seconds = int(float(duration_value.strip()))
            except Exception:
                duration_seconds = None

        # Fallback when Qobuz does not provide explicit release type.
        # Use both track count and duration to avoid classifying long albums as EP.
        if tracks_count <= 3 and (duration_seconds is None or duration_seconds <= 1200):
            return "Single"
        if tracks_count <= 6 and (duration_seconds is None or duration_seconds <= 2100):
            return "EP"
        return "Album"

    return "Release"


def _normalized_text(value):
    return re.sub(r"\s+", " ", str(value or "").strip()).lower()


def _as_dict_list(value):
    if isinstance(value, dict):
        return [value]
    if isinstance(value, list):
        return [item for item in value if isinstance(item, dict)]
    return []


def should_enrich_with_album_meta(album):
    if album.get("product_type") or album.get("release_type"):
        return False

    duration_value = album.get("duration")
    duration_seconds = None
    if isinstance(duration_value, (int, float)):
        duration_seconds = int(duration_value)
    elif isinstance(duration_value, str):
        try:
            duration_seconds = int(float(duration_value.strip()))
        except Exception:
            duration_seconds = None

    # If duration is already present, fallback classification can decide quickly.
    if duration_seconds is not None:
        return False

    tracks_count = album.get("tracks_count")
    if isinstance(tracks_count, int):
        # Only enrich borderline short releases when duration is missing.
        return 4 <= tracks_count <= 8
    return False


def enrich_album_metadata(client, album):
    album_id = str(album.get("id", "")).strip()
    if not album_id:
        return album

    try:
        details = client.get_album_meta(album_id)
    except Exception:
        return album

    if not isinstance(details, dict):
        return album

    merged = dict(album)
    keys_to_backfill = (
        "product_type",
        "release_type",
        "genre",
        "genres_list",
        "duration",
        "tracks_count",
        "artist",
        "main_artist",
        "artists",
        "contributors",
        "performers",
        "artist_id",
        "main_artist_id",
    )

    for key in keys_to_backfill:
        current_value = merged.get(key)
        if current_value in (None, "", [], {}):
            incoming = details.get(key)
            if incoming not in (None, "", [], {}):
                merged[key] = incoming

    return merged


def album_matches_artist(album, expected_artist_id, expected_artist_name):
    expected_id = str(expected_artist_id or "").strip()
    expected_name = _normalized_text(expected_artist_name)

    candidate_ids = set()

    def add_candidate_id(raw_value):
        value = str(raw_value or "").strip()
        if value:
            candidate_ids.add(value)

    add_candidate_id(album.get("artist_id"))
    add_candidate_id(album.get("main_artist_id"))

    for key in ("artist", "main_artist", "performer", "composer", "author", "reader", "narrator"):
        for candidate in _as_dict_list(album.get(key)):
            add_candidate_id(candidate.get("id"))

    for key in ("artists", "contributors"):
        for candidate in _as_dict_list(album.get(key)):
            add_candidate_id(candidate.get("id"))

    if expected_id and expected_id in candidate_ids:
        return True
    if expected_id and candidate_ids:
        return False

    if not expected_name:
        return False

    candidate_names = set()

    def add_candidate_name(raw_value):
        value = _normalized_text(raw_value)
        if value:
            candidate_names.add(value)

    for key in ("artist", "main_artist", "performer", "composer", "author", "reader", "narrator"):
        for candidate in _as_dict_list(album.get(key)):
            add_candidate_name(candidate.get("name"))

    for key in ("artists", "contributors"):
        for candidate in _as_dict_list(album.get(key)):
            add_candidate_name(candidate.get("name"))

    if expected_name in candidate_names:
        return True

    performers = _normalized_text(album.get("performers"))
    if performers and expected_name in performers and ("mainartist" in performers or "artist" in performers):
        return True

    return False


def resolve_cover_url(album):
    image = album.get("image")
    if isinstance(image, str):
        value = image.strip()
        if value.startswith("http"):
            return value
    elif isinstance(image, dict):
        for key in ("extralarge", "large", "mega", "small", "thumbnail"):
            value = str(image.get(key) or "").strip()
            if value.startswith("http"):
                return value

    for key in ("image", "cover", "cover_url", "image_large", "image_small", "image_thumbnail"):
        value = str(album.get(key) or "").strip()
        if value.startswith("http"):
            return value

    return None


def main():
    if len(sys.argv) < 2:
        raise SystemExit(2)

    artist_id = str(sys.argv[1] or "").strip()
    if not artist_id:
        raise SystemExit(2)

    client = load_client()
    pages = [page for page in client.get_artist_meta(artist_id)]
    if not pages:
        print(JSON_MARKER + json.dumps({"artist_name": "Artiste inconnu", "albums": []}, ensure_ascii=False))
        return

    albums = []
    for page in pages:
        if not isinstance(page, dict):
            continue
        page_albums = (page.get("albums") or {}).get("items", [])
        if isinstance(page_albums, list):
            albums.extend(page_albums)

    artist_name = str((pages[0] or {}).get("name") or "Artiste inconnu").strip() or "Artiste inconnu"
    seen = set()
    output_albums = []

    max_enrichment_calls = 24
    enrichment_calls = 0

    for raw_album in albums:
        if not isinstance(raw_album, dict):
            continue

        candidate_album = raw_album
        if should_enrich_with_album_meta(raw_album) and enrichment_calls < max_enrichment_calls:
            enrichment_calls += 1
            candidate_album = enrich_album_metadata(client, raw_album)

        if not album_matches_artist(candidate_album, artist_id, artist_name):
            continue

        album_id = str(candidate_album.get("id", "")).strip()
        if not album_id or album_id in seen:
            continue
        seen.add(album_id)

        title = str(candidate_album.get("title") or "").strip() or f"Album {album_id}"
        version = str(candidate_album.get("version") or "").strip()
        if version:
            title = f"{title} ({version})"

        output_albums.append(
            {
                "id": album_id,
                "title": title,
                "artist_name": (candidate_album.get("artist", {}) or {}).get("name") or artist_name,
                "url": f"https://play.qobuz.com/album/{album_id}",
                "release_timestamp": candidate_album.get("released_at"),
                "tracks_count": candidate_album.get("tracks_count"),
                "release_kind": classify_release(candidate_album),
                "is_hires": bool(candidate_album.get("hires_streamable")),
                "cover_url": resolve_cover_url(candidate_album),
            }
        )

    output_albums.sort(key=lambda item: (item.get("release_timestamp") or 0), reverse=True)

    print(JSON_MARKER + json.dumps({"artist_name": artist_name, "albums": output_albums}, ensure_ascii=False))


if __name__ == "__main__":
    run_with_qobuz_error_handling(main)
