#!/usr/bin/env python3
import configparser
import json
import os
import sys

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


def resolve_tracks(album):
    tracks = album.get("tracks")
    if isinstance(tracks, dict):
        items = tracks.get("items")
        if isinstance(items, list):
            return items
    if isinstance(tracks, list):
        return tracks
    return []


def main():
    if len(sys.argv) < 2:
        raise SystemExit(2)
    album_id = str(sys.argv[1] or "").strip()
    if not album_id:
        raise SystemExit(3)

    client = load_client()
    album = client.get_album_meta(album_id)
    if not isinstance(album, dict):
        raise SystemExit(12)

    entries = []
    for idx, raw in enumerate(resolve_tracks(album), start=1):
        if not isinstance(raw, dict):
            continue
        track_id = str(raw.get("id") or raw.get("track_id") or "").strip() or f"track-{idx}"
        title = str(raw.get("title") or "").strip() or f"Titre {idx}"
        version = str(raw.get("version") or "").strip()
        if version:
            title = f"{title} ({version})"

        entries.append(
            {
                "id": track_id,
                "track_number": parse_int(raw.get("track_number") or raw.get("track_number_in_album") or raw.get("position"), allow_zero=False),
                "title": title,
                "duration_seconds": parse_int(raw.get("duration"), allow_zero=True),
            }
        )

    print(JSON_MARKER + json.dumps({"tracks": entries}, ensure_ascii=False))


if __name__ == "__main__":
    main()
