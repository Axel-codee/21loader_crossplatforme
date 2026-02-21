#!/usr/bin/env python3
"""
Inspect and install Argos Translate language packages for PersoDL.
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Dict, Iterable, List, Sequence, Set, Tuple

BEGIN_MARKER = "PERSODL_ARGOS_JSON_BEGIN"
END_MARKER = "PERSODL_ARGOS_JSON_END"


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Manage Argos language packages.")
    parser.add_argument("--action", required=True, choices=["list", "install"], help="Action to execute.")
    parser.add_argument("--from-code", help="Source language code for install action.")
    parser.add_argument("--to-code", help="Target language code for install action.")
    return parser.parse_args()


def _normalize_code(raw: str) -> str:
    return str(raw or "").strip().lower().replace("_", "-")


def _load_argos_modules():
    try:
        import argostranslate.package as package_mod
        import argostranslate.translate as translate_mod
    except ModuleNotFoundError as exc:
        raise RuntimeError(
            "Python module 'argostranslate' not found. Install it from the app Diagnostics panel."
        ) from exc
    except Exception as exc:
        raise RuntimeError(f"argostranslate import failed: {exc}") from exc
    return package_mod, translate_mod


def _emit(payload: dict) -> None:
    print(BEGIN_MARKER)
    print(json.dumps(payload, ensure_ascii=False))
    print(END_MARKER)


def _find_language(installed_languages: Iterable, code: str):
    normalized = _normalize_code(code)
    for language in installed_languages:
        lang_code = _normalize_code(getattr(language, "code", ""))
        if lang_code == normalized:
            return language
    return None


def _installed_pairs(installed_languages: Sequence) -> Set[Tuple[str, str]]:
    pairs: Set[Tuple[str, str]] = set()
    for source in installed_languages:
        source_code = _normalize_code(getattr(source, "code", ""))
        if not source_code:
            continue
        for target in installed_languages:
            target_code = _normalize_code(getattr(target, "code", ""))
            if not target_code or source_code == target_code:
                continue
            try:
                translation = source.get_translation(target)
            except Exception:
                continue
            if translation is not None:
                pairs.add((source_code, target_code))
    return pairs


def _available_packages(package_mod, refresh_index: bool, warnings: List[str], provided_packages: Sequence | None = None):
    if provided_packages is not None:
        return list(provided_packages)

    if refresh_index:
        try:
            package_mod.update_package_index()
        except Exception as exc:
            warnings.append(f"Impossible de mettre à jour l'index Argos: {exc}")

    try:
        return list(package_mod.get_available_packages())
    except Exception as exc:
        warnings.append(f"Impossible de lire les paquets Argos disponibles: {exc}")
        return []


def _safe_name(raw_name: str, code: str) -> str:
    name = str(raw_name or "").strip()
    return name if name else code


def _build_catalog(package_mod, translate_mod, refresh_index: bool, provided_packages: Sequence | None = None) -> dict:
    warnings: List[str] = []
    installed_languages = list(translate_mod.get_installed_languages())
    packages = _available_packages(package_mod, refresh_index, warnings, provided_packages)

    names_by_code: Dict[str, str] = {}
    installed_codes: Set[str] = set()

    for language in installed_languages:
        code = _normalize_code(getattr(language, "code", ""))
        if not code:
            continue
        installed_codes.add(code)
        language_name = str(getattr(language, "name", "")).strip()
        if language_name:
            names_by_code[code] = language_name

    available_pairs: Set[Tuple[str, str]] = set()
    for package in packages:
        from_code = _normalize_code(getattr(package, "from_code", ""))
        to_code = _normalize_code(getattr(package, "to_code", ""))
        if not from_code or not to_code:
            continue
        available_pairs.add((from_code, to_code))
        from_name = str(getattr(package, "from_name", "")).strip()
        to_name = str(getattr(package, "to_name", "")).strip()
        if from_name and from_code not in names_by_code:
            names_by_code[from_code] = from_name
        if to_name and to_code not in names_by_code:
            names_by_code[to_code] = to_name

    installed_pairs = _installed_pairs(installed_languages)
    available_pairs.update(installed_pairs)

    all_codes: Set[str] = set(installed_codes)
    for source_code, target_code in available_pairs:
        all_codes.add(source_code)
        all_codes.add(target_code)

    languages = [
        {
            "code": code,
            "name": _safe_name(names_by_code.get(code, ""), code),
            "installed": code in installed_codes,
        }
        for code in sorted(
            all_codes,
            key=lambda item: (_safe_name(names_by_code.get(item, ""), item).casefold(), item),
        )
    ]

    pairs = [
        {
            "sourceCode": source_code,
            "targetCode": target_code,
            "installed": (source_code, target_code) in installed_pairs,
        }
        for source_code, target_code in sorted(available_pairs)
    ]

    return {
        "runtimeAvailable": True,
        "languages": languages,
        "pairs": pairs,
        "warnings": warnings,
    }


def _install_pair(package_mod, translate_mod, from_code: str, to_code: str):
    try:
        package_mod.update_package_index()
        packages = list(package_mod.get_available_packages())
    except Exception as exc:
        raise RuntimeError(
            "Unable to fetch Argos package index. Check internet access for first-time package install."
        ) from exc

    selected_package = None
    for package in packages:
        source_code = _normalize_code(getattr(package, "from_code", ""))
        target_code = _normalize_code(getattr(package, "to_code", ""))
        if source_code == from_code and target_code == to_code:
            selected_package = package
            break
    if selected_package is None:
        raise RuntimeError(f"No Argos package available for {from_code}->{to_code}.")

    try:
        package_path = selected_package.download()
        package_mod.install_from_path(package_path)
    except Exception as exc:
        raise RuntimeError(f"Failed to install Argos package {from_code}->{to_code}.") from exc

    installed_languages = list(translate_mod.get_installed_languages())
    source = _find_language(installed_languages, from_code)
    target = _find_language(installed_languages, to_code)
    if source is None or target is None:
        raise RuntimeError(f"Argos package install completed but language pair {from_code}->{to_code} is unavailable.")
    try:
        translation = source.get_translation(target)
    except Exception as exc:
        raise RuntimeError(f"Unable to initialize Argos translation {from_code}->{to_code}.") from exc
    if translation is None:
        raise RuntimeError(f"Argos package install completed but language pair {from_code}->{to_code} is unavailable.")

    return packages


def _is_pair_installed(translate_mod, from_code: str, to_code: str) -> bool:
    installed_languages = list(translate_mod.get_installed_languages())
    source = _find_language(installed_languages, from_code)
    target = _find_language(installed_languages, to_code)
    if source is None or target is None:
        return False
    try:
        return source.get_translation(target) is not None
    except Exception:
        return False


def main() -> int:
    args = _parse_args()
    package_mod, translate_mod = _load_argos_modules()

    if args.action == "list":
        catalog = _build_catalog(package_mod, translate_mod, refresh_index=True)
        _emit({"ok": True, "message": "Catalogue Argos chargé.", "catalog": catalog})
        return 0

    if args.action == "install":
        source_code = _normalize_code(args.from_code or "")
        target_code = _normalize_code(args.to_code or "")
        if not source_code or not target_code:
            raise RuntimeError("--from-code et --to-code sont requis pour l'action install.")
        if source_code == target_code:
            raise RuntimeError("Langues source/cible identiques.")

        if _is_pair_installed(translate_mod, source_code, target_code):
            message = f"Paquet Argos {source_code}->{target_code} déjà installé."
            catalog = _build_catalog(package_mod, translate_mod, refresh_index=False)
            _emit({"ok": True, "message": message, "catalog": catalog})
            return 0

        packages = _install_pair(package_mod, translate_mod, source_code, target_code)
        message = f"Paquet Argos {source_code}->{target_code} installé."
        catalog = _build_catalog(package_mod, translate_mod, refresh_index=False, provided_packages=packages)
        _emit({"ok": True, "message": message, "catalog": catalog})
        return 0

    raise RuntimeError(f"Action non supportée: {args.action}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr, flush=True)
        raise SystemExit(1)
