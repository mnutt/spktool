#!/usr/bin/env python3
"""Rewrite Sandstorm pkgdef .capnp files via capnp eval/convert."""

import argparse
import json
import os
import re
import subprocess
from pathlib import Path


def _default_import_paths():
    paths = []
    sandstorm_home = os.environ.get("SANDSTORM_HOME")
    if sandstorm_home:
        candidate = Path(sandstorm_home) / "latest" / "usr" / "include"
        if candidate.exists():
            paths.append(str(candidate))
    return paths


def _capnp_base_cmd(import_paths=None):
    cmd = ["capnp"]
    for path in import_paths or _default_import_paths():
        cmd.extend(["-I", path])
    return cmd


def _package_schema_for_convert(import_paths=None):
    paths = list(import_paths or _default_import_paths())
    for base in paths:
        candidate = Path(base) / "sandstorm" / "package.capnp"
        if candidate.exists():
            return str(candidate)
    return "/sandstorm/package.capnp"


def _capnp_file_to_json(capnp_file_path, import_paths=None):
    cmd = _capnp_base_cmd(import_paths) + [
        "eval",
        "-ojson",
        "--short",
        capnp_file_path,
        "pkgdef",
    ]
    proc = subprocess.run(cmd, check=True, capture_output=True, text=True)
    return json.loads(proc.stdout)


def _extract_file_id(capnp_definition_contents):
    match = re.search(r"^\s*@([^;]+);", capnp_definition_contents, flags=re.MULTILINE)
    return match.group(1).strip() if match else None


def _package_definition_json_to_capnp_file_contents(package_definition_json, import_paths=None, file_id=None):
    json_payload = json.dumps(package_definition_json, separators=(",", ":"))
    cmd = _capnp_base_cmd(import_paths) + [
        "convert",
        "--short",
        "json:text",
        _package_schema_for_convert(import_paths),
        "PackageDefinition",
    ]
    proc = subprocess.run(cmd, input=json_payload, check=True, capture_output=True, text=True)
    chosen_file_id = file_id or subprocess.run(
        ["capnp", "id"], check=True, capture_output=True, text=True
    ).stdout.strip()
    return (
        f"@{chosen_file_id};\n\n"
        'using Spk = import "/sandstorm/package.capnp";\n\n'
        f"const pkgdef :Spk.PackageDefinition = {proc.stdout.strip()};\n"
    )


def strip_signing_fields(pkgdef_json):
    signing_keys = {
        "pgpSignature",
        "pgpKeyring",
        "authorPgpSignature",
        "authorPgpKeyring",
        "authorPgpKeyFingerprint",
    }
    if isinstance(pkgdef_json, dict):
        for key in list(pkgdef_json.keys()):
            if key in signing_keys:
                del pkgdef_json[key]
            else:
                strip_signing_fields(pkgdef_json[key])
    elif isinstance(pkgdef_json, list):
        for item in pkgdef_json:
            strip_signing_fields(item)


def set_marketing_version(pkgdef_json, version):
    manifest = pkgdef_json.get("manifest")
    if not isinstance(manifest, dict):
        raise ValueError("Could not find manifest in pkgdef")
    app_marketing = manifest.get("appMarketingVersion")
    if not isinstance(app_marketing, dict):
        raise ValueError("Could not find manifest.appMarketingVersion in pkgdef")
    default_text = app_marketing.get("defaultText")
    if not isinstance(default_text, str):
        raise ValueError("Could not find manifest.appMarketingVersion.defaultText in pkgdef")
    app_marketing["defaultText"] = version


def _append_title_suffix(pkgdef_json, suffix):
    manifest = pkgdef_json.get("manifest")
    app_title = manifest.get("appTitle") if isinstance(manifest, dict) else None
    default_text = app_title.get("defaultText") if isinstance(app_title, dict) else None
    if not isinstance(default_text, str):
        raise ValueError("Could not find manifest.appTitle.defaultText in pkgdef")
    if not default_text.endswith(suffix):
        app_title["defaultText"] = default_text + suffix


def rewrite_pkgdef(input_capnp_file, new_app_id, output_capnp_file=None, append_title_suffix="", strip_signing=False, set_version=None, import_paths=None):
    contents = Path(input_capnp_file).read_text(encoding="utf-8")
    pkgdef_json = _capnp_file_to_json(input_capnp_file, import_paths=import_paths)
    pkgdef_json["id"] = new_app_id

    if strip_signing:
        strip_signing_fields(pkgdef_json)
    if set_version is not None:
        set_marketing_version(pkgdef_json, set_version)
    if append_title_suffix:
        _append_title_suffix(pkgdef_json, append_title_suffix)

    output_contents = _package_definition_json_to_capnp_file_contents(
        pkgdef_json,
        import_paths=import_paths,
        file_id=_extract_file_id(contents),
    )
    if output_capnp_file:
        Path(output_capnp_file).write_text(output_contents, encoding="utf-8")
    return output_contents


def _parse_args():
    parser = argparse.ArgumentParser(description="Rewrite app id/title/signing metadata in a Sandstorm pkgdef .capnp file.")
    parser.add_argument("-I", "--import-path", action="append", default=[])
    parser.add_argument("capnp_file")
    parser.add_argument("new_app_id")
    parser.add_argument("-o", "--output")
    parser.add_argument("--append-title-suffix", default="")
    parser.add_argument("--strip-signing", action="store_true")
    parser.add_argument("--set-version")
    return parser.parse_args()


def main():
    args = _parse_args()
    output = rewrite_pkgdef(
        args.capnp_file,
        args.new_app_id,
        output_capnp_file=args.output,
        append_title_suffix=args.append_title_suffix,
        strip_signing=args.strip_signing,
        set_version=args.set_version,
        import_paths=args.import_path or None,
    )
    if not args.output:
        print(output, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
