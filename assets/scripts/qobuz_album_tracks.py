#!/usr/bin/env python3
import json
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
    run_with_qobuz_error_handling(main)
