"""Check the publishable tree without printing possible credential values."""
import argparse
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def audit():
    names = subprocess.check_output(["git", "ls-files", "-co", "--exclude-standard", "-z"], cwd=ROOT).decode().split("\0")
    findings = []
    patterns = {
        "OpenAI-style credential": re.compile(r"\bsk-(?:proj-)?[A-Za-z0-9_-]{24,}"),
        "Google-style credential": re.compile(r"\bAIza[0-9A-Za-z_-]{30,}"),
        "private key": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    }
    forbidden = ("data/", "chars/", "papers/", "design-preview/", "output/", ".env", "web/node_modules/", "build/")
    for name in sorted(set(names)):
        file = ROOT/name
        if not name or not file.is_file():
            continue
        if name.startswith(forbidden) or file.name == "secrets.json":
            findings.append({"path": name, "reason": "private/generated directory in source manifest"})
            continue
        if file.suffix.lower() in (".png", ".jpg", ".jpeg", ".webp", ".gif"):
            findings.append({"path": name, "reason": "raster asset requires a documented license"})
        try:
            content = file.read_text(encoding="utf-8-sig")
        except (ValueError, UnicodeDecodeError):
            continue
        for kind, expression in patterns.items():
            for match in expression.finditer(content):
                findings.append({"path": name, "line": content.count("\n", 0, match.start())+1, "reason": kind})
    for required in ("LICENSE", "CONTRIBUTING.md", "SECURITY.md", "THIRD_PARTY_NOTICES.md", "docs/ASSET_LICENSES.md", "docs/OPERATIONS.md"):
        if not (ROOT/required).is_file():
            findings.append({"path": required, "reason": "required release document missing"})
    return {"passed": not findings, "findings": findings,
            "scope": "current tracked and nonignored source tree; Git history and private output are excluded",
            "historyPublication": "Use the clean source archive or resolve historical material rights before publishing Git history."}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--strict", action="store_true")
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()
    report = audit()
    raw = json.dumps(report, ensure_ascii=False, indent=2)
    if args.out:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(raw, encoding="utf-8")
    print(raw)
    if args.strict and not report["passed"]:
        raise SystemExit(1)
