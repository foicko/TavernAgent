#!/usr/bin/env python3
"""分层门禁：禁止 Go 包反向依赖（见 AGENTS.md、docs/PROJECT_STATUS.md §2）。

规则：`domain` ← `ports` ← `context`/`application`/`pack` ← `adapters`。
`internal/textutil` 是仅依赖标准库的叶子包，任何层可 import。

用法：`python scripts/layering_check.py`（CI 的 Go 步骤会调用）。
"""
import subprocess
import sys

MODULE = "tavernagent/internal/"

# key = 包（internal/ 下第一段）；value = 该包**禁止** import 的 internal 包前缀
RULES = {
    "domain": ["application", "adapters", "context", "ports", "protocol"],
    "ports": ["application", "adapters", "context", "protocol"],
    "context": ["application", "adapters"],
    "pack": ["application", "adapters"],
    "protocol": ["application", "adapters"],
    "textutil": ["application", "adapters", "context", "domain", "pack", "ports", "protocol"],
}


def main() -> int:
    out = subprocess.check_output(
        ["go", "list", "-f", '{{.ImportPath}}|{{join .Imports " "}}', "./..."],
        text=True,
    )
    violations: list[str] = []
    for line in out.splitlines():
        pkg, _, imports = line.partition("|")
        if not pkg.startswith(MODULE):
            continue
        short = pkg[len(MODULE):].split("/")[0]
        banned = RULES.get(short)
        if not banned:
            continue
        for imp in imports.split():
            if not imp.startswith(MODULE):
                continue
            target = imp[len(MODULE):].split("/")[0]
            if target in banned:
                violations.append(f"{pkg} -> {imp}")
    if violations:
        print("layering violations:")
        for item in violations:
            print("  " + item)
        return 1
    print("layering OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
