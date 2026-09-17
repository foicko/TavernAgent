"""架构门禁：文件/函数规模、圈复杂度与分层依赖（借鉴 Reasonix 的 repolint）。

为什么需要：`sqlite.go` 1878 行、`styles.css` 6671 行、`compile.go` 1600+ 行这类
"再改就得先读懂一屏"的文件，靠人工 review 拦不住继续变长。这里把它变成 CI 检查，
并配一份 **baseline 棘轮**：只拦新增违规，不要求一次性清掉历史债（Reasonix 自己的
controller.go 有 5896 行，也没靠这套规则清零，它拦的是"再长"）。

用法：
    python scripts/architecture_check.py              # 检查（新增或变大都失败）
    python scripts/architecture_check.py --update     # 收紧 baseline（只许持平或变小）
    python scripts/architecture_check.py --list       # 打印全部违规明细

棘轮语义：baseline 记录每条违规的**数值**。已基线的文件可以继续存在，但
"继续变长/变复杂"必须失败——只比 key 不比数值的话，1879 行的文件长到 4000 行
也照样通过，历史债就成了免检区。放宽（数值增长或新增条目）必须显式
--accept-growth，并在提交信息里写明理由。
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BASELINE = ROOT / "scripts/architecture_baseline.json"

MAX_FILE_LINES = 800
MAX_FUNC_LINES = 120
MAX_COMPLEXITY = 30

# 分层白名单：包路径前缀 → 不得导入的内部包前缀。
LAYER_RULES = {
    "internal/domain": ("tavernagent/internal/adapters", "tavernagent/internal/application", "tavernagent/internal/adapters/http"),
    "internal/ports": ("tavernagent/internal/adapters", "tavernagent/internal/application", "tavernagent/internal/context"),
    "internal/context": ("tavernagent/internal/adapters", "tavernagent/internal/application"),
    "internal/application": ("tavernagent/internal/adapters",),
    "internal/protocol": ("tavernagent/internal/adapters", "tavernagent/internal/application"),
    "internal/search": ("tavernagent/internal/adapters", "tavernagent/internal/application"),
    "internal/pack": ("tavernagent/internal/adapters", "tavernagent/internal/application"),
}

# 注意：&& 与 || 是非单词字符，不能套 \b（那样永远匹配不上）。
BRANCH_KEYWORDS = re.compile(r"\b(?:if|for|case)\b|&&|\|\|")


def strip_go_source(text: str) -> str:
    """去掉注释与字符串字面量，避免把模板里的花括号当成代码块。

    注释要**保留换行**：函数边界与行数都按行计算，吞掉换行会让后面的函数
    被错并成一行（曾把 contentDisposition 算成"1 行 94 复杂度"）。
    """
    out = []
    i = 0
    while i < len(text):
        ch = text[i]
        nxt = text[i + 1] if i + 1 < len(text) else ""
        if ch == "/" and nxt == "/":
            end = text.find("\n", i)
            if end < 0:
                break
            out.append("\n")
            i = end + 1
            continue
        if ch == "/" and nxt == "*":
            end = text.find("*/", i + 2)
            skipped = text[i:] if end < 0 else text[i:end + 2]
            out.append("\n" * skipped.count("\n"))
            i = len(text) if end < 0 else end + 2
            continue
        if ch == "'":
            # rune 字面量：必须先于双引号分支处理，否则 `'"'` 会被当成字符串开头，
            # 一路吞到下一个双引号（曾把 contentDisposition 的收尾花括号吃掉，
            # 函数被错算成 1 行 94 复杂度）。
            out.append("''")
            cursor = i + 1
            closed = False
            while cursor < len(text):
                if text[cursor] == "\\":
                    cursor += 2
                    continue
                if text[cursor] == "'":
                    cursor += 1
                    closed = True
                    break
                cursor += 1
            # 未闭合（孤立引号/畸形字面量）时只跳过这一格，绝不吞掉文件其余部分。
            i = cursor if closed else i + 1
            continue
        if ch == '"':
            i += 1
            while i < len(text) and text[i] != '"':
                i += 2 if text[i] == "\\" else 1
            i += 1
            out.append('""')
            continue
        if ch == "`":
            end = text.find("`", i + 1)
            skipped = text[i:] if end < 0 else text[i:end + 1]
            out.append("\n" * skipped.count("\n"))
            i = len(text) if end < 0 else end + 1
            continue
        out.append(ch)
        i += 1
    return "".join(out)


def go_functions(stripped: str) -> list[tuple[str, int, int, int]]:
    """返回 (函数名, 起始行, 行数, 圈复杂度)。"""
    lines = stripped.split("\n")
    functions = []
    index = 0
    while index < len(lines):
        line = lines[index]
        match = re.match(r"func\s+(?:\([^)]*\)\s*)?([A-Za-z_][\w]*)\s*\(", line)
        if not match:
            index += 1
            continue
        name = match.group(1)
        depth = 0
        started = False
        end = index
        body_lines = []
        for cursor in range(index, len(lines)):
            segment = lines[cursor]
            depth += segment.count("{") - segment.count("}")
            if "{" in segment:
                started = True
            body_lines.append(segment)
            if started and depth <= 0:
                end = cursor
                break
        body = "\n".join(body_lines)
        complexity = 1 + len(BRANCH_KEYWORDS.findall(body))
        functions.append((name, index + 1, end - index + 1, complexity))
        index = end + 1
    return functions


def go_imports(text: str) -> list[str]:
    """提取非测试文件里的内部包导入。"""
    imports = []
    for match in re.finditer(r'^\s*(?:[\w.]+\s+)?"(tavernagent/internal/[^"]+)"', text, re.M):
        imports.append(match.group(1))
    return imports


def check_go_file(path: Path, violations: list[dict]) -> None:
    rel = path.relative_to(ROOT).as_posix()
    text = path.read_text(encoding="utf-8", errors="replace")
    line_count = text.count("\n") + 1
    if line_count > MAX_FILE_LINES:
        violations.append({"rule": "file-lines", "path": rel, "value": line_count, "limit": MAX_FILE_LINES})
    for name, start, length, complexity in go_functions(strip_go_source(text)):
        if length > MAX_FUNC_LINES:
            violations.append({"rule": "func-lines", "path": rel, "symbol": name, "value": length, "limit": MAX_FUNC_LINES})
        if complexity > MAX_COMPLEXITY:
            violations.append({"rule": "complexity", "path": rel, "symbol": name, "value": complexity, "limit": MAX_COMPLEXITY})
    if path.name.endswith("_test.go"):
        return
    violations.extend(layer_violations(rel, text))


def layer_violations(rel: str, text: str) -> list[dict]:
    """分层检查：只约束非测试的生产代码（测试夹具允许直接起真实存储）。"""
    found = []
    for prefix, forbidden in LAYER_RULES.items():
        if not rel.startswith(prefix + "/") and rel != prefix:
            continue
        for imported in go_imports(text):
            if imported.startswith(forbidden):
                found.append({"rule": "layering", "path": rel, "symbol": imported, "value": 0, "limit": 0})
    return found


def check_frontend_file(path: Path, violations: list[dict]) -> None:
    rel = path.relative_to(ROOT).as_posix()
    if "__tests__" in rel or ".test." in rel:
        return
    line_count = path.read_text(encoding="utf-8", errors="replace").count("\n") + 1
    if line_count > MAX_FILE_LINES:
        violations.append({"rule": "file-lines", "path": rel, "value": line_count, "limit": MAX_FILE_LINES})


def collect() -> list[dict]:
    violations: list[dict] = []
    for path in sorted((ROOT / "internal").rglob("*.go")):
        check_go_file(path, violations)
    for path in sorted((ROOT / "cmd").rglob("*.go")):
        check_go_file(path, violations)
    for pattern in ("*.ts", "*.tsx", "*.css"):
        for path in sorted((ROOT / "web/src").rglob(pattern)):
            check_frontend_file(path, violations)
    return violations


def key_of(violation: dict) -> str:
    return "|".join(str(violation.get(field, "")) for field in ("rule", "path", "symbol"))


def load_baseline() -> dict:
    if not BASELINE.is_file():
        return {}
    return json.loads(BASELINE.read_text(encoding="utf-8")).get("violations", {})


def compare(violations: list[dict], known: dict) -> dict:
    """把当前违规与基线对照，分成新增 / 增长 / 收紧三类。"""
    added: list[dict] = []
    grown: list[tuple[dict, int]] = []
    shrunk: list[tuple[dict, int]] = []
    for violation in violations:
        baseline = known.get(key_of(violation))
        if baseline is None:
            added.append(violation)
            continue
        was = baseline.get("value", 0)
        if violation["value"] > was:
            grown.append((violation, was))
        elif violation["value"] < was:
            shrunk.append((violation, was))
    return {"added": added, "grown": grown, "shrunk": shrunk}


def describe(violation: dict) -> str:
    symbol = ":" + violation["symbol"] if violation.get("symbol") else ""
    return f"{violation['rule']} {violation['path']}{symbol}"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--update", action="store_true", help="把当前违规写为新的 baseline（只许持平或收紧）")
    parser.add_argument("--accept-growth", action="store_true", help="允许 --update 放宽基线（新增或变大）")
    parser.add_argument("--list", action="store_true", help="打印全部违规明细")
    args = parser.parse_args()

    violations = collect()
    if args.list:
        for violation in violations:
            print(f"{violation['rule']:12s} {violation['path']}"
                  f"{(':' + violation['symbol']) if violation.get('symbol') else ''}"
                  f" = {violation['value']}")
        print(f"共 {len(violations)} 条违规")
        return 0

    report = compare(violations, load_baseline())

    if args.update:
        relaxed = report["added"] + [violation for violation, _ in report["grown"]]
        if relaxed and not args.accept_growth:
            print("--update 只能持平或收紧，以下条目会让基线变宽：")
            for violation in report["added"]:
                print(f"  ✗ 新增 {describe(violation)} = {violation['value']}")
            for violation, was in report["grown"]:
                print(f"  ✗ 增长 {describe(violation)} = {violation['value']}（基线 {was}）")
            print("\n放宽基线请显式加 --accept-growth，并在提交信息里写明理由。")
            return 1
        baseline = {key_of(v): {"value": v["value"]} for v in violations}
        BASELINE.write_text(json.dumps({"violations": baseline}, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
        widened = f"；其中放宽 {len(relaxed)} 条（--accept-growth）" if relaxed else ""
        print(f"已写入 {BASELINE.relative_to(ROOT)}（{len(baseline)} 条{widened}）")
        return 0

    if report["added"]:
        print("新增结构违规（baseline 里没有）：")
        for violation in report["added"]:
            print(f"  ✗ {describe(violation)} = {violation['value']}（上限 {violation['limit']}）")
        print("\n如果这是有意的，请先与评审沟通，再用 --update --accept-growth 记录理由；"
              "不要用 --update 一次性吞掉所有违规。")
        return 1
    if report["grown"]:
        print("已基线的违规继续变大（基线是棘轮：只许持平或变小）：")
        for violation, was in report["grown"]:
            print(f"  ✗ {describe(violation)} = {violation['value']}（基线 {was}，上限 {violation['limit']}）")
        print("\n请拆分或收敛；确实无法避免时用 --update --accept-growth 并注明理由。")
        return 1
    if report["shrunk"]:
        print(f"提示：{len(report['shrunk'])} 条已低于基线，可运行 --update 收紧棘轮。")
    print(f"架构门禁通过：违规 {len(violations)} 条，均在 baseline 内且未变大。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
