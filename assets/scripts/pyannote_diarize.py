#!/usr/bin/env python3

import argparse
import json
import os
import sys


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Local pyannote diarization wrapper")
    parser.add_argument("--audio", required=True, help="Input WAV path")
    parser.add_argument("--output-json", required=True, help="Normalized output JSON path")
    parser.add_argument("--token", default="", help="Hugging Face access token (prefer PYANNOTE_HF_TOKEN)")
    parser.add_argument("--pipeline-path", default="", help="Optional local pipeline directory")
    return parser.parse_args()


def disable_telemetry() -> None:
    os.environ["PYANNOTE_METRICS_ENABLED"] = "0"
    os.environ["HF_HUB_DISABLE_TELEMETRY"] = "1"
    os.environ["DO_NOT_TRACK"] = "1"


def annotation_to_segments(annotation) -> list[dict]:
    if annotation is None:
        return []

    segments = []
    if hasattr(annotation, "itertracks"):
        iterator = annotation.itertracks(yield_label=True)
        for segment, _track, label in iterator:
            segments.append(
                {
                    "start": round(float(segment.start), 6),
                    "end": round(float(segment.end), 6),
                    "speaker": str(label),
                }
            )
        return segments

    for item in annotation:
        if len(item) == 2:
            segment, label = item
        elif len(item) == 3:
            segment, _track, label = item
        else:
            continue
        segments.append(
            {
                "start": round(float(segment.start), 6),
                "end": round(float(segment.end), 6),
                "speaker": str(label),
            }
        )
    return segments


def compute_duration(*segment_lists: list[dict]) -> float:
    duration = 0.0
    for segments in segment_lists:
        for segment in segments:
            duration = max(duration, float(segment.get("end", 0.0) or 0.0))
    return round(duration, 6)


def main() -> int:
    args = parse_args()
    disable_telemetry()

    try:
        from pyannote.audio import Pipeline

        try:
            from pyannote.audio.telemetry import set_telemetry_metrics

            set_telemetry_metrics(False)
        except Exception:
            pass

        source = args.pipeline_path.strip() or "pyannote/speaker-diarization-community-1"
        token = (args.token or os.environ.get("PYANNOTE_HF_TOKEN", "")).strip()
        kwargs = {}
        if token and not args.pipeline_path.strip():
            kwargs["token"] = token

        print("[pyannote] Chargement du pipeline...", flush=True)
        pipeline = Pipeline.from_pretrained(source, **kwargs)

        print("[pyannote] Diarisation locale...", flush=True)
        output = pipeline(args.audio)

        diarization = getattr(output, "speaker_diarization", output)
        exclusive = getattr(output, "exclusive_speaker_diarization", None)
        segments = annotation_to_segments(diarization)
        exclusive_segments = annotation_to_segments(exclusive)
        speaker_labels = sorted({segment["speaker"] for segment in segments + exclusive_segments if segment.get("speaker")})

        payload = {
            "pipeline": source,
            "duration": compute_duration(segments, exclusive_segments),
            "speaker_labels": speaker_labels,
            "segments": segments,
            "exclusive_segments": exclusive_segments,
        }

        os.makedirs(os.path.dirname(os.path.abspath(args.output_json)), exist_ok=True)
        with open(args.output_json, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, ensure_ascii=True, indent=2)
            handle.write("\n")

        print(f"[pyannote] JSON ecrit: {args.output_json}", flush=True)
        return 0
    except Exception as exc:
        print(f"[pyannote] Erreur: {exc}", file=sys.stderr, flush=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
