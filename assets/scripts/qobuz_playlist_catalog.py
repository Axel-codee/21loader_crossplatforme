#!/usr/bin/env python3
import json
import re
import sys

sys.dont_write_bytecode = True

from qobuz_common import load_client, run_with_qobuz_error_handling

JSON_MARKER = "__LOADER21_QOBUZ_JSON__"


def parse_int(value, allow_zero=False):
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        if value > 0 or (allow_zero and value >= 0):
            return value
        return None
    if isinstance(value, float):
        parsed = int(round(value))
        if parsed > 0 or (allow_zero and parsed >= 0):
            return parsed
        return None
    if isinstance(value, str):
        raw = value.strip()
        if not raw:
            return None
        try:
            parsed = int(float(raw))
            if parsed > 0 or (allow_zero and parsed >= 0):
                return parsed
        except Exception:
            return None
    return None


def normalize_text(value, max_length=0):
    text = re.sub(r"\s+", " ", str(value or "").strip())
    if max_length > 0 and len(text) > max_length:
        text = text[:max_length].rstrip()
    return text


def as_dict_list(value):
    if isinstance(value, dict):
        return [value]
    if isinstance(value, list):
        return [item for item in value if isinstance(item, dict)]
    return []


def resolve_cover_url(entity):
    if not isinstance(entity, dict):
        return None

    image = entity.get("image")
    if isinstance(image, str):
        value = image.strip()
        if value.startswith("http"):
            return value
    elif isinstance(image, dict):
        for key in ("extralarge", "large", "mega", "small", "thumbnail"):
            value = str(image.get(key) or "").strip()
            if value.startswith("http"):
                return value

    for key in ("cover", "cover_url", "image_large", "image_small", "image_thumbnail", "picture", "picture_url"):
        value = str(entity.get(key) or "").strip()
        if value.startswith("http"):
            return value
    return None


def resolve_tracks(page):
    tracks = page.get("tracks")
    if isinstance(tracks, dict):
        items = tracks.get("items")
        if isinstance(items, list):
            return items
    if isinstance(tracks, list):
        return tracks
    return []


def make_artist_url(artist_id):
    artist_id = str(artist_id or "").strip()
    if not artist_id:
        return ""
    return f"https://play.qobuz.com/artist/{artist_id}"


def make_album_url(album_id):
    album_id = str(album_id or "").strip()
    if not album_id:
        return ""
    return f"https://play.qobuz.com/album/{album_id}"


def resolve_artist_info(track, album):
    candidates = []

    for key in ("performer", "artist", "main_artist", "composer", "author", "reader", "narrator"):
        candidates.extend(as_dict_list(track.get(key)))
    for key in ("artists", "contributors"):
        candidates.extend(as_dict_list(track.get(key)))
    for key in ("artist", "main_artist", "performer"):
        candidates.extend(as_dict_list(album.get(key)))
    for key in ("artists", "contributors"):
        candidates.extend(as_dict_list(album.get(key)))

    selected = None
    for candidate in candidates:
        cid = str(candidate.get("id") or "").strip()
        name = normalize_text(candidate.get("name"), 220)
        if cid or name:
            selected = candidate
            break

    if selected is None:
        cid = str(track.get("artist_id") or album.get("artist_id") or album.get("main_artist_id") or "").strip()
        name = normalize_text(track.get("artist_name") or album.get("artist_name"), 220)
        if cid or name:
            return {"id": cid, "name": name or f"Artiste {cid}", "url": make_artist_url(cid)}
        return {"id": "", "name": "", "url": ""}

    artist_id = str(selected.get("id") or "").strip()
    artist_name = normalize_text(selected.get("name"), 220)
    artist_url = str(selected.get("url") or selected.get("web_url") or "").strip()
    if not artist_url:
        artist_url = make_artist_url(artist_id)
    return {"id": artist_id, "name": artist_name, "url": artist_url}


def resolve_playlist_name(first_page, playlist_id):
    for key in ("name", "title"):
        value = normalize_text(first_page.get(key), 220)
        if value:
            return value
    owner = first_page.get("owner")
    if isinstance(owner, dict):
        owner_name = normalize_text(owner.get("name"), 160)
        if owner_name:
            return f"Playlist {owner_name}"
    return f"Playlist {playlist_id}"


def build_album_entry(album_id, album_title, artist_info, album):
    release_timestamp = parse_int(album.get("released_at"), allow_zero=False)
    track_count = parse_int(album.get("tracks_count"), allow_zero=True)
    return {
        "id": album_id,
        "title": album_title or f"Album {album_id}",
        "artist_id": artist_info.get("id", ""),
        "artist_name": artist_info.get("name", "") or "Artiste inconnu",
        "url": make_album_url(album_id),
        "release_timestamp": release_timestamp,
        "tracks_count": track_count,
        "release_kind": "Release",
        "is_hires": bool(album.get("hires_streamable")),
        "cover_url": resolve_cover_url(album) or "",
        "tracks_in_playlist": 0,
    }


def main():
    if len(sys.argv) < 2:
        raise SystemExit(2)
    playlist_id = str(sys.argv[1] or "").strip()
    if not playlist_id:
        raise SystemExit(3)

    client = load_client()
    pages = [page for page in client.get_plist_meta(playlist_id) if isinstance(page, dict)]

    if not pages:
        output = {
            "playlist_id": playlist_id,
            "playlist_name": f"Playlist {playlist_id}",
            "url": f"https://play.qobuz.com/playlist/{playlist_id}",
            "tracks_count": 0,
            "tracks": [],
            "albums": [],
            "artists": [],
        }
        print(JSON_MARKER + json.dumps(output, ensure_ascii=False))
        return

    first_page = pages[0]
    playlist_name = resolve_playlist_name(first_page, playlist_id)
    declared_tracks_count = parse_int(first_page.get("tracks_count"), allow_zero=True)
    playlist_url = f"https://play.qobuz.com/playlist/{playlist_id}"

    track_entries = []
    albums_by_id = {}
    artists_by_key = {}
    running_index = 1

    for page in pages:
        for raw_track in resolve_tracks(page):
            if not isinstance(raw_track, dict):
                continue

            track_id = str(raw_track.get("id") or raw_track.get("track_id") or "").strip() or f"track-{running_index}"
            track_title = normalize_text(raw_track.get("title"), 240) or f"Titre {running_index}"
            version = normalize_text(raw_track.get("version"), 120)
            if version:
                track_title = f"{track_title} ({version})"

            track_position = parse_int(raw_track.get("position"), allow_zero=False) or running_index
            duration_seconds = parse_int(raw_track.get("duration"), allow_zero=True)

            album = raw_track.get("album")
            if not isinstance(album, dict):
                album = {}

            album_id = str(album.get("id") or raw_track.get("album_id") or "").strip()
            album_title = normalize_text(album.get("title"), 240)
            album_version = normalize_text(album.get("version"), 120)
            if album_title and album_version:
                album_title = f"{album_title} ({album_version})"
            if not album_title and album_id:
                album_title = f"Album {album_id}"

            artist_info = resolve_artist_info(raw_track, album)
            artist_id = str(artist_info.get("id") or "").strip()
            artist_name = normalize_text(artist_info.get("name"), 220) or "Artiste inconnu"
            artist_url = str(artist_info.get("url") or "").strip() or make_artist_url(artist_id)

            album_url = make_album_url(album_id)
            track_entries.append(
                {
                    "id": track_id,
                    "position": track_position,
                    "title": track_title,
                    "duration_seconds": duration_seconds,
                    "artist_id": artist_id,
                    "artist_name": artist_name,
                    "artist_url": artist_url,
                    "album_id": album_id,
                    "album_title": album_title,
                    "album_url": album_url,
                }
            )

            if album_id:
                if album_id not in albums_by_id:
                    albums_by_id[album_id] = build_album_entry(album_id, album_title, {"id": artist_id, "name": artist_name}, album)
                albums_by_id[album_id]["tracks_in_playlist"] += 1

            artist_key = artist_id if artist_id else f"name:{artist_name.lower()}"
            if artist_key not in artists_by_key:
                artists_by_key[artist_key] = {
                    "id": artist_id,
                    "name": artist_name,
                    "url": artist_url,
                    "tracks_in_playlist": 0,
                    "album_ids": set(),
                }
            artists_by_key[artist_key]["tracks_in_playlist"] += 1
            if album_id:
                artists_by_key[artist_key]["album_ids"].add(album_id)

            running_index += 1

    albums = list(albums_by_id.values())
    albums.sort(key=lambda item: (str(item.get("artist_name", "")).lower(), str(item.get("title", "")).lower(), str(item.get("id", "")).lower()))

    artists = []
    for artist in artists_by_key.values():
        artists.append(
            {
                "id": artist["id"],
                "name": artist["name"],
                "url": artist["url"],
                "tracks_in_playlist": artist["tracks_in_playlist"],
                "albums_in_playlist": len(artist["album_ids"]),
            }
        )
    artists.sort(key=lambda item: (-int(item.get("tracks_in_playlist") or 0), str(item.get("name", "")).lower()))

    output = {
        "playlist_id": playlist_id,
        "playlist_name": playlist_name,
        "url": playlist_url,
        "tracks_count": declared_tracks_count if declared_tracks_count is not None else len(track_entries),
        "tracks": track_entries,
        "albums": albums,
        "artists": artists,
    }

    print(JSON_MARKER + json.dumps(output, ensure_ascii=False))


if __name__ == "__main__":
    run_with_qobuz_error_handling(main)
