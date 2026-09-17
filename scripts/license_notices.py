"""Collect installed runtime dependency license texts; fail on missing notices."""
import json
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def documents(directory):
    return sorted({file for pattern in ("LICENSE*", "LICENCE*", "COPYING*", "NOTICE*", "UNLICENSE*") for file in directory.glob(pattern) if file.is_file()})


def collect():
    output = ROOT/"LICENSES"
    output.mkdir(exist_ok=True)
    rows, missing = [], []
    def add(name, version, directory, declared="See included text"):
        files = documents(Path(directory))
        if not files:
            missing.append(name)
            return
        stem = re.sub(r"[^A-Za-z0-9._-]", "_", name)
        links = []
        for index, file in enumerate(files):
            destination = output/f"{stem}-{index+1}.txt"
            destination.write_bytes(file.read_bytes())
            links.append(f"[{file.name}](LICENSES/{destination.name})")
        rows.append(f"| {name} | {version} | {declared} | {' · '.join(links)} |")
    raw = subprocess.check_output(["go", "list", "-m", "-json", "all"], cwd=ROOT, text=True)
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        module, end = decoder.raw_decode(raw)
        raw = raw[end:]
        if not module.get("Main"):
            add(module["Path"], module["Version"], module["Dir"])
    add("Go standard library", subprocess.check_output(["go", "env", "GOVERSION"], text=True).strip(), subprocess.check_output(["go", "env", "GOROOT"], text=True).strip(), "BSD-3-Clause")
    pnpm = shutil.which("pnpm")
    if not pnpm:
        raise RuntimeError("pnpm required")
    packages = json.loads(subprocess.check_output([pnpm, "licenses", "list", "--prod", "--json"], cwd=ROOT/"web", text=True, encoding="utf-8"))
    for license_name, dependencies in packages.items():
        for dependency in dependencies:
            add(dependency["name"], ", ".join(dependency["versions"]), dependency["paths"][0], license_name)
    if missing:
        raise RuntimeError("Missing license texts: " + ", ".join(missing))
    (ROOT/"THIRD_PARTY_NOTICES.md").write_text("# Third-party dependency notices\n\nGenerated conservatively from all locked Go modules and the installed production frontend dependency graph. Some Go modules support development only; listing a license here does not imply that component is linked into every artifact. The original license texts govern their respective components; the project MIT license does not replace them. Build and test tools are not bundled in the executable.\n\n| Component | Version | License | Included text |\n| --- | --- | --- | --- |\n" + "\n".join(rows) + "\n\nOriginal story assets are listed in [ASSET_LICENSES.md](docs/ASSET_LICENSES.md). SQLite itself is public domain; its Go translation and supporting libraries retain the notices listed above.\n", encoding="utf-8")
    print(f"Collected {len(rows)} dependency notices")


if __name__ == "__main__":
    collect()
