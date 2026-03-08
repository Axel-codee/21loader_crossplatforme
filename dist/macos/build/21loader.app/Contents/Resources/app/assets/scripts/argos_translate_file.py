#!/usr/bin/env python3
"""
Translate text or subtitle files with Argos Translate.

This script is designed for offline/local usage in 21loader jobs.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


_SRT_INDEX_RE = re.compile(r"^\s*\d+\s*$")


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Translate a file with Argos Translate.")
    parser.add_argument("--input", required=True, help="Input file path.")
    parser.add_argument("--output", required=True, help="Output file path.")
    parser.add_argument("--format", required=True, choices=["txt", "srt"], help="Input format.")
    parser.add_argument("--from-code", required=True, help="Source language code (example: en).")
    parser.add_argument("--to-code", required=True, help="Target language code (example: fr).")
    parser.add_argument(
        "--no-auto-install",
        action="store_true",
        help="Disable auto-install of missing Argos language package.",
    )
    return parser.parse_args()


def _load_argos_modules():
    try:
        import argostranslate.package as package_mod
        import argostranslate.translate as translate_mod
    except ModuleNotFoundError as exc:  # pragma: no cover - depends on runtime env.
        raise RuntimeError(
            "Python module 'argostranslate' not found. "
            "Install it from the app Diagnostics panel (Argos dependency)."
        ) from exc
    except Exception as exc:  # pragma: no cover - depends on runtime env.
        raise RuntimeError(f"argostranslate import failed: {exc}") from exc
    return package_mod, translate_mod


def _find_language(installed_languages, code: str):
    normalized = (code or "").strip().lower().replace("_", "-")
    for language in installed_languages:
        lang_code = str(getattr(language, "code", "")).strip().lower().replace("_", "-")
        if lang_code == normalized:
            return language
    return None


def _ensure_translation(translate_mod, package_mod, from_code: str, to_code: str, auto_install: bool):
    installed = translate_mod.get_installed_languages()
    source = _find_language(installed, from_code)
    target = _find_language(installed, to_code)

    if source is not None and target is not None:
        try:
            return source.get_translation(target)
        except Exception:
            pass

    if not auto_install:
        raise RuntimeError(
            f"Argos language package {from_code}->{to_code} is not installed. "
            "Re-run without --no-auto-install or install the package manually."
        )

    try:
        package_mod.update_package_index()
        packages = package_mod.get_available_packages()
    except Exception as exc:
        raise RuntimeError(
            "Unable to fetch Argos package index. Check internet access for first-time package install."
        ) from exc

    source_normalized = from_code.strip().lower().replace("_", "-")
    target_normalized = to_code.strip().lower().replace("_", "-")
    selected = None
    for package in packages:
        from_lang = str(getattr(package, "from_code", "")).strip().lower().replace("_", "-")
        to_lang = str(getattr(package, "to_code", "")).strip().lower().replace("_", "-")
        if from_lang == source_normalized and to_lang == target_normalized:
            selected = package
            break

    if selected is None:
        raise RuntimeError(f"No Argos package available for {from_code}->{to_code}.")

    try:
        package_path = selected.download()
        package_mod.install_from_path(package_path)
    except Exception as exc:
        raise RuntimeError(f"Failed to install Argos package {from_code}->{to_code}.") from exc

    installed = translate_mod.get_installed_languages()
    source = _find_language(installed, from_code)
    target = _find_language(installed, to_code)
    if source is None or target is None:
        raise RuntimeError(f"Argos package install completed but language pair {from_code}->{to_code} is unavailable.")
    try:
        return source.get_translation(target)
    except Exception as exc:
        raise RuntimeError(f"Unable to initialize Argos translation {from_code}->{to_code}.") from exc


def _translate_text_line(line: str, translator) -> str:
    if not line:
        return line
    leading_len = len(line) - len(line.lstrip())
    trailing_len = len(line) - len(line.rstrip())
    core = line.strip()
    if not core:
        return line
    translated = translator.translate(core)
    leading = line[:leading_len]
    trailing = line[len(line) - trailing_len :] if trailing_len > 0 else ""
    return f"{leading}{translated}{trailing}"


def _emit_argos_progress(fmt_name: str, done: int, total: int, state: dict) -> None:
    done = max(0, int(done))
    total = max(0, int(total))
    if total <= 0:
        pct = 100
        done = 0
    else:
        if done > total:
            done = total
        pct = int((done * 100) / total)
        if done >= total:
            pct = 100
    last_pct = int(state.get("last_pct", -1))
    if pct == last_pct:
        return
    if pct < 100 and last_pct >= 0 and (pct - last_pct) < 2:
        return
    state["last_pct"] = pct
    print(f"[argos] {fmt_name}: {pct}% ({done}/{total})", file=sys.stderr, flush=True)


def _translate_txt(content: str, translator) -> str:
    lines = content.splitlines()
    total = sum(1 for line in lines if line.strip())
    progress_state = {"last_pct": -1}
    _emit_argos_progress("txt", 0, total, progress_state)
    translated_lines = []
    done = 0
    for line in lines:
        if not line.strip():
            translated_lines.append(line)
            continue
        translated_lines.append(_translate_text_line(line, translator))
        done += 1
        _emit_argos_progress("txt", done, total, progress_state)
    out = "\n".join(translated_lines)
    if content.endswith("\n"):
        out += "\n"
    _emit_argos_progress("txt", total, total, progress_state)
    return out


def _looks_like_srt_timecode(line: str) -> bool:
    return "-->" in line


def _collect_srt_text_spans(lines: list[str]) -> list[tuple[int, int]]:
    spans: list[tuple[int, int]] = []
    idx = 0
    total = len(lines)
    while idx < total:
        while idx < total and not lines[idx].strip():
            idx += 1
        if idx >= total:
            break

        if (
            _SRT_INDEX_RE.match(lines[idx].strip())
            and idx + 1 < total
            and _looks_like_srt_timecode(lines[idx + 1])
        ):
            idx += 1

        if idx >= total or not _looks_like_srt_timecode(lines[idx]):
            while idx < total and lines[idx].strip():
                idx += 1
            continue

        idx += 1
        text_start = idx
        while idx < total and lines[idx].strip():
            idx += 1
        text_end = idx
        if text_end > text_start:
            spans.append((text_start, text_end))
    return spans


def _compact_subtitle_text(lines: list[str]) -> str:
    return " ".join(part.strip() for part in lines if part.strip())


def _wrap_subtitle_text(text: str, max_chars: int = 42) -> list[str]:
    compact = " ".join(text.split())
    if not compact:
        return [text]
    words = compact.split(" ")
    wrapped: list[str] = []
    current = words[0]
    for word in words[1:]:
        if len(current) + 1 + len(word) <= max_chars:
            current = f"{current} {word}"
            continue
        wrapped.append(current)
        current = word
    wrapped.append(current)
    return wrapped


def _translate_srt(content: str, translator) -> str:
    lines = content.splitlines()
    spans = _collect_srt_text_spans(lines)
    total = len(spans)
    progress_state = {"last_pct": -1}
    _emit_argos_progress("srt", 0, total, progress_state)
    done = 0
    offset = 0
    for start, end in spans:
        start_adj = start + offset
        end_adj = end + offset
        cue_text = _compact_subtitle_text(lines[start_adj:end_adj])
        translated_cue = translator.translate(cue_text) if cue_text else cue_text
        replacement = _wrap_subtitle_text(translated_cue)
        lines[start_adj:end_adj] = replacement
        offset += len(replacement) - (end - start)
        done += 1
        _emit_argos_progress("srt", done, total, progress_state)
    out = "\n".join(lines)
    if content.endswith("\n"):
        out += "\n"
    _emit_argos_progress("srt", total, total, progress_state)
    return out


def main() -> int:
    args = _parse_args()
    from_code = args.from_code.strip().lower().replace("_", "-")
    to_code = args.to_code.strip().lower().replace("_", "-")

    if not from_code or not to_code:
        print("from-code and to-code are required.", file=sys.stderr)
        return 2
    if from_code == to_code:
        print("from-code and to-code must be different.", file=sys.stderr)
        return 2

    input_path = Path(args.input)
    output_path = Path(args.output)

    try:
        content = input_path.read_text(encoding="utf-8")
    except Exception as exc:
        print(f"Unable to read input file: {input_path} ({exc})", file=sys.stderr)
        return 2

    try:
        package_mod, translate_mod = _load_argos_modules()
        translator = _ensure_translation(
            translate_mod,
            package_mod,
            from_code,
            to_code,
            auto_install=not args.no_auto_install,
        )
        if args.format == "srt":
            translated = _translate_srt(content, translator)
        else:
            translated = _translate_txt(content, translator)
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        return 1

    try:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(translated, encoding="utf-8")
    except Exception as exc:
        print(f"Unable to write output file: {output_path} ({exc})", file=sys.stderr)
        return 2

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
