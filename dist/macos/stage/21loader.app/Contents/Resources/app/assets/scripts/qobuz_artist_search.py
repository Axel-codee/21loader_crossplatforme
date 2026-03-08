#!/usr/bin/env python3
import configparser
import json
import os
import re
import sys
import ast
import html

from qobuz_dl.qopy import Client

JSON_MARKER = "__LOADER21_QOBUZ_JSON__"


def load_client():
    config_path = os.path.join(os.path.expanduser("~"), ".config", "qobuz-dl", "config.ini")
    config = configparser.ConfigParser()
    if not config.read(config_path):
        raise SystemExit(10)

    default = config["DEFAULT"]
    email = default.get("email", "").strip()
    password = default.get("password", "").strip()
    app_id = default.get("app_id", "").strip()
    secrets = [secret for secret in default.get("secrets", "").split(",") if secret]
    if not email or not password or not app_id or not secrets:
        raise SystemExit(11)
    return Client(email, password, app_id, secrets)


def parse_limit(raw_value):
    if raw_value is None:
        return 12
    value = str(raw_value).strip()
    if not value:
        return 12
    try:
        parsed = int(float(value))
    except Exception:
        return 12
    if parsed <= 0:
        return 12
    if parsed > 50:
        return 50
    return parsed


def parse_int(value):
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value if value >= 0 else None
    if isinstance(value, float):
        parsed = int(round(value))
        return parsed if parsed >= 0 else None
    if isinstance(value, str):
        raw = value.strip()
        if not raw:
            return None
        try:
            parsed = int(float(raw))
            return parsed if parsed >= 0 else None
        except Exception:
            return None
    return None


def normalized_text(value, max_length=0):
    text = re.sub(r"\s+", " ", str(value or "").strip())
    if max_length > 0 and len(text) > max_length:
        text = text[:max_length].rstrip()
    return text


def clean_biography_text(value, max_length=1400):
    text = str(value or "")
    text = html.unescape(text)
    text = text.replace("\\n", " ")
    text = re.sub(r"<br\s*/?>", " ", text, flags=re.IGNORECASE)
    text = re.sub(r"</p\s*>", " ", text, flags=re.IGNORECASE)
    text = re.sub(r"<[^>]+>", " ", text)
    text = re.sub(r"\s+", " ", text).strip()
    if max_length > 0 and len(text) > max_length:
        text = text[:max_length].rstrip()
    return text


def parse_biography_payload(value):
    if isinstance(value, dict):
        return value
    if not isinstance(value, str):
        return None
    raw = value.strip()
    if not raw:
        return None
    if ("summary" not in raw.lower()) and ("content" not in raw.lower()):
        return None

    try:
        parsed = json.loads(raw)
        if isinstance(parsed, dict):
            return parsed
    except Exception:
        pass

    try:
        parsed = ast.literal_eval(raw)
        if isinstance(parsed, dict):
            return parsed
    except Exception:
        pass

    return None


def choose_summary_or_content(summary, content):
    summary_clean = clean_biography_text(summary)
    content_clean = clean_biography_text(content)
    if summary_clean and content_clean:
        summary_norm = summary_clean.lower()
        content_norm = content_clean.lower()
        if content_norm.startswith(summary_norm):
            return summary_clean
        if len(summary_clean) <= len(content_clean) and summary_norm in content_norm:
            return summary_clean
        return content_clean
    if summary_clean:
        return summary_clean
    return content_clean


def resolve_cover_url(artist):
    image = artist.get("image")
    if isinstance(image, str):
        value = image.strip()
        if value.startswith("http"):
            return value
    elif isinstance(image, dict):
        for key in ("extralarge", "large", "mega", "small", "thumbnail"):
            value = str(image.get(key) or "").strip()
            if value.startswith("http"):
                return value

    for key in ("image", "image_url", "picture", "picture_url", "portrait", "portrait_url"):
        value = str(artist.get(key) or "").strip()
        if value.startswith("http"):
            return value

    return None


def extract_artist_items(payload):
    if isinstance(payload, list):
        return [item for item in payload if isinstance(item, dict)]
    if isinstance(payload, dict):
        nested = payload.get("artists")
        if isinstance(nested, dict):
            items = nested.get("items")
            if isinstance(items, list):
                return [item for item in items if isinstance(item, dict)]
        items = payload.get("items")
        if isinstance(items, list):
            return [item for item in items if isinstance(item, dict)]
    return []


def extract_genres(artist):
    values = []

    genres_list = artist.get("genres_list")
    if isinstance(genres_list, list):
        for raw in genres_list:
            if isinstance(raw, dict):
                values.append(raw.get("name"))
            else:
                values.append(raw)

    genre = artist.get("genre")
    if isinstance(genre, dict):
        values.append(genre.get("name"))
    else:
        values.append(genre)

    genres = artist.get("genres")
    if isinstance(genres, list):
        for raw in genres:
            if isinstance(raw, dict):
                values.append(raw.get("name"))
            else:
                values.append(raw)

    unique = []
    seen = set()
    for value in values:
        item = normalized_text(value)
        key = item.lower()
        if not item or key in seen:
            continue
        seen.add(key)
        unique.append(item)
    return unique


def extract_country(artist):
    for key in ("country", "country_code", "nationality", "area", "location"):
        value = normalized_text(artist.get(key))
        if value:
            return value
    return ""


def extract_biography(artist):
    for key in ("biography", "bio", "description", "short_biography", "presentation", "about"):
        raw_value = artist.get(key)
        payload = parse_biography_payload(raw_value)
        if isinstance(payload, dict):
            summary = payload.get("summary") or payload.get("short") or payload.get("title")
            content = payload.get("content") or payload.get("text") or payload.get("description")
            value = choose_summary_or_content(summary, content)
            if value:
                return value
        value = clean_biography_text(raw_value, 1400)
        if value:
            return value
    return ""


def merge_artist_metadata(primary, secondary):
    merged = dict(primary)
    for key in (
        "name",
        "slug",
        "country",
        "country_code",
        "nationality",
        "area",
        "location",
        "genres_list",
        "genres",
        "genre",
        "biography",
        "bio",
        "description",
        "short_biography",
        "presentation",
        "about",
        "image",
        "image_url",
        "portrait",
        "portrait_url",
        "albums_count",
        "albums",
    ):
        if merged.get(key) in (None, "", [], {}):
            incoming = secondary.get(key)
            if incoming not in (None, "", [], {}):
                merged[key] = incoming
    return merged


def first_artist_page(client, artist_id):
    try:
        pages = client.get_artist_meta(artist_id)
        for page in pages:
            if isinstance(page, dict):
                return page
            break
    except Exception:
        return None
    return None


def extract_latest_release(payload):
    albums = []
    if isinstance(payload, dict):
        albums_raw = (payload.get("albums") or {}).get("items")
        if isinstance(albums_raw, list):
            albums = [item for item in albums_raw if isinstance(item, dict)]

    best_title = ""
    best_timestamp = None
    for album in albums:
        timestamp = parse_int(album.get("released_at"))
        if timestamp is None:
            continue
        title = normalized_text(album.get("title"))
        version = normalized_text(album.get("version"))
        if version:
            title = f"{title} ({version})" if title else version
        if best_timestamp is None or timestamp > best_timestamp:
            best_timestamp = timestamp
            best_title = title

    return best_title, best_timestamp


def main():
    if len(sys.argv) < 2:
        raise SystemExit(2)

    query = normalized_text(sys.argv[1])
    if not query:
        raise SystemExit(2)
    limit = parse_limit(sys.argv[2] if len(sys.argv) > 2 else None)

    client = load_client()
    raw_results = client.search_artists(query, limit)
    items = extract_artist_items(raw_results)

    artists = []
    seen_ids = set()
    max_enrichment_calls = min(limit, 12)
    enrichment_calls = 0

    for raw in items:
        artist_id = normalized_text(raw.get("id"))
        if not artist_id or artist_id in seen_ids:
            continue
        seen_ids.add(artist_id)

        first_page = None
        if enrichment_calls < max_enrichment_calls:
            enrichment_calls += 1
            first_page = first_artist_page(client, artist_id)

        merged = merge_artist_metadata(raw, first_page or {})
        name = normalized_text(merged.get("name")) or f"Artiste {artist_id}"
        albums_count = parse_int(raw.get("albums_count"))

        catalog_albums_count = None
        if isinstance(first_page, dict):
            catalog_albums_count = parse_int(first_page.get("albums_count"))
            if catalog_albums_count is None:
                catalog_albums_count = parse_int((first_page.get("albums") or {}).get("total"))

        if albums_count is None:
            albums_count = catalog_albums_count

        genres = extract_genres(merged)
        biography = extract_biography(merged)
        country = extract_country(merged)
        latest_release_title, latest_release_timestamp = extract_latest_release(first_page or merged)
        image_url = resolve_cover_url(merged) or resolve_cover_url(raw)
        slug = normalized_text(merged.get("slug"))

        artists.append(
            {
                "id": artist_id,
                "name": name,
                "url": f"https://play.qobuz.com/artist/{artist_id}",
                "albums_count": albums_count,
                "catalog_albums_count": catalog_albums_count,
                "image_url": image_url,
                "slug": slug,
                "country": country,
                "genres": genres,
                "biography": biography,
                "latest_release_title": latest_release_title,
                "latest_release_timestamp": latest_release_timestamp,
            }
        )

    print(JSON_MARKER + json.dumps({"artists": artists}, ensure_ascii=False))


if __name__ == "__main__":
    main()
