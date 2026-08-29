#!/usr/bin/env python3
"""Validate website/static/recommended-providers/{zh-CN,en}.json against the
catalog schema enforced by the Go `CatalogHandler`.

Runs in three modes:
  --all (default)      validate every .json file in the directory
  --file <path>        validate a single file
Exit codes: 0 = clean, 1 = validation errors found, 2 = usage error.

Schema rules (mirror the server-side annotation logic so we never ship an
invalid catalog to the running gateway):
  - top-level: version (string, semver-ish yyyy-mm-dd), updated_at (RFC3339),
    bundles (non-empty array)
  - bundle: id (slug), title, description, providers (non-empty array)
  - provider: provider (string), models (array of strings), apply_url
    (string, http/https or empty for keyless), apply_steps (array), is_keyless
    (bool), notes (string), optional base_provider / base_url
  - when base_provider is present:
      * it must be one of the supported base provider protocols used by
        custom providers (openai / anthropic / cohere / gemini / bedrock /
        replicate / huggingface)
      * base_url is required and must be a syntactically valid http/https URL
  - uniqueness: bundle.id unique within a file; provider unique within a
    bundle; provider unique across all bundles (no global duplicates)
  - cross-file parity: every bundle.id and every provider.name that exists
    in zh-CN.json must exist with the same set in en.json (and vice versa) —
    language files must stay in sync, since the server picks one per request
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any, Dict, List, Tuple
from urllib.parse import urlparse

# Mirrors core/schemas/bifrost.go SupportedBaseProviders. If you change this
# list, also change the Go side and update the docs.
SUPPORTED_BASE_PROVIDERS = {
    "openai",
    "anthropic",
    "cohere",
    "gemini",
    "bedrock",
    "replicate",
    "huggingface",
}

DEFAULT_DIR = Path("website/static/recommended-providers")
VERSION_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
RFC3339_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$")
SLUG_RE = re.compile(r"^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?$")


def _err(errors: List[str], where: str, msg: str) -> None:
    errors.append(f"{where}: {msg}")


def _validate_provider(p: Any, where: str, errors: List[str]) -> Tuple[str, bool]:
    """Return (provider_name, has_base_fallback)."""
    if not isinstance(p, dict):
        _err(errors, where, "provider entry must be an object")
        return "<invalid>", False

    name = p.get("provider")
    if not isinstance(name, str) or not name:
        _err(errors, where, "provider.name must be a non-empty string")
        name = "<invalid>"
    elif not re.match(r"^[a-z0-9_.-]+$", name):
        _err(errors, where, f"provider.name {name!r} must be lowercase slug (a-z, 0-9, _, -, .)")

    if not isinstance(p.get("models", []), list):
        _err(errors, where, "provider.models must be an array of strings")
    else:
        for i, m in enumerate(p["models"]):
            if not isinstance(m, str) or not m:
                _err(errors, f"{where}.models[{i}]", "model must be a non-empty string")

    apply_url = p.get("apply_url", "")
    if not isinstance(apply_url, str):
        _err(errors, where, "provider.apply_url must be a string")
    elif apply_url:
        parsed = urlparse(apply_url)
        if parsed.scheme not in ("http", "https") or not parsed.netloc:
            _err(errors, where, f"provider.apply_url must be a valid http(s) URL, got {apply_url!r}")

    if not isinstance(p.get("apply_steps", []), list):
        _err(errors, where, "provider.apply_steps must be an array of strings")
    else:
        for i, s in enumerate(p["apply_steps"]):
            if not isinstance(s, str) or not s:
                _err(errors, f"{where}.apply_steps[{i}]", "step must be a non-empty string")

    if not isinstance(p.get("is_keyless", False), bool):
        _err(errors, where, "provider.is_keyless must be a boolean")

    if not isinstance(p.get("notes", ""), str):
        _err(errors, where, "provider.notes must be a string")

    free_until = p.get("free_valid_until")
    if free_until is not None:
        if not isinstance(free_until, str) or not re.match(r"^\d{4}-\d{2}-\d{2}$", free_until):
            _err(errors, where, f"provider.free_valid_until must be a yyyy-mm-dd date, got {free_until!r}")

    has_base = False
    base = p.get("base_provider")
    base_url = p.get("base_url")
    if base is not None or base_url is not None:
        has_base = True
        if base is None or base not in SUPPORTED_BASE_PROVIDERS:
            _err(errors, where, f"provider.base_provider must be one of {sorted(SUPPORTED_BASE_PROVIDERS)}, got {base!r}")
        if not isinstance(base_url, str) or not base_url:
            _err(errors, where, "provider.base_url is required when base_provider is present")
        else:
            parsed = urlparse(base_url)
            if parsed.scheme not in ("http", "https") or not parsed.netloc:
                _err(errors, where, f"provider.base_url must be a valid http(s) URL, got {base_url!r}")

    return name, has_base


def _validate_bundle(b: Any, where: str, errors: List[str]) -> Tuple[str, List[Tuple[str, bool]]]:
    if not isinstance(b, dict):
        _err(errors, where, "bundle must be an object")
        return "<invalid>", []

    bid = b.get("id")
    if not isinstance(bid, str) or not SLUG_RE.match(bid):
        _err(errors, where, f"bundle.id must be a lowercase slug, got {bid!r}")
        bid = "<invalid>"

    if not isinstance(b.get("title", ""), str) or not b.get("title"):
        _err(errors, where, "bundle.title must be a non-empty string")
    if not isinstance(b.get("description", ""), str) or not b.get("description"):
        _err(errors, where, "bundle.description must be a non-empty string")

    providers_raw = b.get("providers", [])
    if not isinstance(providers_raw, list) or not providers_raw:
        _err(errors, where, "bundle.providers must be a non-empty array")
        return bid, []

    out: List[Tuple[str, bool]] = []
    seen_in_bundle: set[str] = set()
    for i, p in enumerate(providers_raw):
        name, has_base = _validate_provider(p, f"{where}.providers[{i}]", errors)
        if name in seen_in_bundle and name != "<invalid>":
            _err(errors, f"{where}.providers[{i}]", f"duplicate provider {name!r} within bundle {bid!r}")
        seen_in_bundle.add(name)
        out.append((name, has_base))

    return bid, out


def _validate_file(path: Path, errors: List[str]) -> Dict[str, List[Tuple[str, bool]]] | None:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        _err(errors, str(path), "file not found")
        return None
    except json.JSONDecodeError as e:
        _err(errors, str(path), f"invalid JSON: {e.msg} at line {e.lineno}")
        return None

    if not isinstance(data, dict):
        _err(errors, str(path), "top-level must be an object")
        return None

    version = data.get("version")
    if not isinstance(version, str) or not VERSION_RE.match(version):
        _err(errors, str(path), f"top-level.version must match yyyy-mm-dd, got {version!r}")

    updated_at = data.get("updated_at")
    if not isinstance(updated_at, str) or not RFC3339_RE.match(updated_at):
        _err(errors, str(path), f"top-level.updated_at must be RFC3339, got {updated_at!r}")

    bundles_raw = data.get("bundles", [])
    if not isinstance(bundles_raw, list) or not bundles_raw:
        _err(errors, str(path), "top-level.bundles must be a non-empty array")
        return None

    by_bundle: Dict[str, List[Tuple[str, bool]]] = {}
    seen_bundle_ids: set[str] = set()
    for i, b in enumerate(bundles_raw):
        bid, providers = _validate_bundle(b, f"{path}#bundles[{i}]", errors)
        if bid in seen_bundle_ids and bid != "<invalid>":
            _err(errors, f"{path}#bundles[{i}]", f"duplicate bundle.id {bid!r}")
        seen_bundle_ids.add(bid)
        by_bundle[bid] = providers

    return by_bundle


def _check_parity(
    summaries: Dict[str, Dict[str, List[Tuple[str, bool]]]],
    errors: List[str],
) -> None:
    """Enforce that zh-CN and en files stay in sync (same bundle ids and same
    provider sets per bundle)."""
    keys = sorted(summaries.keys())
    if len(keys) < 2:
        return

    base = keys[0]
    for other in keys[1:]:
        b_bundles = set(summaries[base].keys())
        o_bundles = set(summaries[other].keys())
        only_b = b_bundles - o_bundles
        only_o = o_bundles - b_bundles
        for bid in only_b:
            _err(errors, f"parity({base} vs {other})", f"bundle {bid!r} only in {base}")
        for bid in only_o:
            _err(errors, f"parity({base} vs {other})", f"bundle {bid!r} only in {other}")
        for bid in b_bundles & o_bundles:
            b_provs = {p for p, _ in summaries[base][bid]}
            o_provs = {p for p, _ in summaries[other][bid]}
            for prov in b_provs - o_provs:
                _err(errors, f"parity({base} vs {other})", f"provider {prov!r} in bundle {bid!r} only in {base}")
            for prov in o_provs - b_provs:
                _err(errors, f"parity({base} vs {other})", f"provider {prov!r} in bundle {bid!r} only in {other}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--dir", type=Path, default=DEFAULT_DIR, help="directory containing catalog JSON files")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--all", action="store_true", help="validate every *.json in --dir (default)")
    group.add_argument("--file", type=Path, help="validate a single JSON file")
    args = parser.parse_args()

    if args.file:
        files = [args.file]
    else:
        if not args.dir.exists():
            print(f"error: directory {args.dir} does not exist", file=sys.stderr)
            return 2
        files = sorted(args.dir.glob("*.json"))
        if not files:
            print(f"error: no .json files found in {args.dir}", file=sys.stderr)
            return 2

    errors: List[str] = []
    summaries: Dict[str, Dict[str, List[Tuple[str, bool]]]] = {}
    for path in files:
        rel = path.name if path.is_absolute() else str(path)
        summary = _validate_file(path, errors)
        if summary is not None:
            summaries[rel] = summary

    _check_parity(summaries, errors)

    if errors:
        print(f"FAIL: {len(errors)} validation error(s):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    files_label = ", ".join(sorted(summaries.keys()))
    print(f"OK: {len(summaries)} file(s) validated — {files_label}")
    return 0


if __name__ == "__main__":
    sys.exit(main())