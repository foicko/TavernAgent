"""架构门禁的纯函数回归：扫描器不能误报，也不能漏报。

不依赖真实仓库内容——用合成片段锁定行为，避免"门禁自己坏掉却没人发现"。
"""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import architecture_check as ac


class StripSourceTests(unittest.TestCase):
    def test_ignores_braces_and_keywords_inside_strings_and_comments(self):
        code = '''
// 这句注释里有 if 和 { 花括号
func demo() {
	text := "if 字符串里的 { 不算代码"
	raw := `{"kind":"narration"}`
	/* 块注释里的 case { */
	_ = text
	_ = raw
}
'''
        stripped = ac.strip_go_source(code)
        self.assertNotIn("注释里有 if", stripped)
        self.assertNotIn('"if 字符串里', stripped)
        self.assertNotIn("块注释里的 case", stripped)

    def test_function_length_ignores_trailing_code(self):
        code = "func a() {\n\tx := 1\n\t_ = x\n}\n\nfunc b() {\n\t_ = 2\n}\n"
        funcs = ac.go_functions(ac.strip_go_source(code))
        self.assertEqual([f[0] for f in funcs], ["a", "b"])
        self.assertEqual(funcs[0][2], 4)
        self.assertEqual(funcs[1][2], 3)

    def test_complexity_counts_branches(self):
        code = """func decide(x int) int {
	if x > 1 && x < 10 {
		return 1
	}
	for i := 0; i < x; i++ {
		if i%2 == 0 {
			return 2
		}
	}
	switch x {
	case 3:
		return 3
	}
	return 0
}
"""
        funcs = ac.go_functions(ac.strip_go_source(code))
        # 1 + if + && + for + if + case = 6
        self.assertEqual(funcs[0][3], 6)

    def test_methods_with_receivers_are_measured(self):
        code = "func (s *Store) Save(a int) error {\n\treturn nil\n}\n"
        funcs = ac.go_functions(ac.strip_go_source(code))
        self.assertEqual(funcs[0][0], "Save")
        self.assertEqual(funcs[0][2], 3)


class LayeringTests(unittest.TestCase):
    def test_domain_importing_adapters_is_flagged(self):
        text = 'package domain\n\nimport "tavernagent/internal/adapters/sqlite"\n'
        found = ac.layer_violations("internal/domain/state.go", text)
        self.assertEqual(len(found), 1)
        self.assertEqual(found[0]["rule"], "layering")
        self.assertIn("adapters/sqlite", found[0]["symbol"])

    def test_application_importing_context_is_allowed(self):
        text = 'package application\n\nimport (\n\tctxpkg "tavernagent/internal/context"\n)\n'
        self.assertEqual(ac.layer_violations("internal/application/service.go", text), [])

    def test_application_importing_adapters_is_flagged(self):
        text = 'package application\n\nimport "tavernagent/internal/adapters/providers/openai"\n'
        found = ac.layer_violations("internal/application/providers.go", text)
        self.assertEqual(len(found), 1)

    def test_rules_only_apply_to_matching_package(self):
        text = 'package sqlite\n\nimport "tavernagent/internal/application"\n'
        self.assertEqual(ac.layer_violations("internal/adapters/sqlite/sqlite.go", text), [])


class BaselineKeyTests(unittest.TestCase):
    def test_key_is_stable_across_value_changes(self):
        first = {"rule": "file-lines", "path": "a.go", "value": 900, "limit": 800}
        second = {"rule": "file-lines", "path": "a.go", "value": 1200, "limit": 800}
        self.assertEqual(ac.key_of(first), ac.key_of(second))

    def test_key_distinguishes_symbols(self):
        first = {"rule": "func-lines", "path": "a.go", "symbol": "A", "value": 200}
        second = {"rule": "func-lines", "path": "a.go", "symbol": "B", "value": 200}
        self.assertNotEqual(ac.key_of(first), ac.key_of(second))


if __name__ == "__main__":
    unittest.main()


class StripLineFidelityTests(unittest.TestCase):
    """strip 必须与原文逐行对应，否则函数边界与行数会整体错位。"""

    def test_preserves_line_count_for_every_comment_kind(self):
        code = (
            "package p\n"
            "\n"
            "// 行注释\n"
            "func a() {\n"
            "\t/* 块注释\n"
            "\t   跨两行 */\n"
            "\tx := 1\n"
            "\t_ = x\n"
            "}\n"
        )
        stripped = ac.strip_go_source(code)
        self.assertEqual(stripped.count("\n"), code.count("\n"))

    def test_rune_literal_with_quote_does_not_swallow_following_code(self):
        code = "\n".join([
            "package p",
            "",
            "func contentDisposition(name string) string {",
            "	ascii := make([]rune, 0, len(name))",
            "	for _, r := range name {",
            '''		if r < 128 && r != '"' && r != '\\\\' {''',
            "			ascii = append(ascii, r)",
            "		}",
            "	}",
            "	return string(ascii)",
            "}",
            "",
            "func after() int {",
            '	if len("x") > 0 {',
            "		return 1",
            "	}",
            "	return 0",
            "}",
            "",
        ])
        self.assertIn("'\\\\'", code)  # 样本是合法 Go：反斜杠 rune 字面量
        self.assertEqual(code.count("\n"), ac.strip_go_source(code).count("\n"))
        functions = {f[0]: f for f in ac.go_functions(ac.strip_go_source(code))}
        self.assertEqual(list(functions), ["contentDisposition", "after"])
        self.assertEqual(functions["contentDisposition"][2], 9)
        self.assertEqual(functions["after"][2], 6)

    def test_unterminated_rune_literal_does_not_swallow_the_rest_of_the_file(self):
        code = "\n".join([
            "package p",
            "",
            "func broken() {",
            "	x := '\\'",
            "}",
            "",
            "func after() {}",
            "",
        ])
        stripped = ac.strip_go_source(code)
        self.assertEqual(stripped.count("\n"), code.count("\n"))
        self.assertIn("func after()", stripped)


class RatchetComparisonTests(unittest.TestCase):
    """棘轮语义：新增与变大必须失败，持平通过，变小可收紧。"""

    def violation(self, value, path="internal/a.go", symbol="F"):
        return {"rule": "func-lines", "path": path, "symbol": symbol, "value": value, "limit": 120}

    def test_new_and_grown_are_both_flagged(self):
        known = {ac.key_of(self.violation(100)): {"value": 100}}
        report = ac.compare([self.violation(101), self.violation(10, path="internal/b.go")], known)
        self.assertEqual(len(report["grown"]), 1)
        self.assertEqual(len(report["added"]), 1)
        self.assertEqual(report["shrunk"], [])

    def test_equal_and_smaller_pass(self):
        known = {
            ac.key_of(self.violation(100)): {"value": 100},
            ac.key_of(self.violation(90, symbol="G")): {"value": 90},
        }
        report = ac.compare([self.violation(100), self.violation(80, symbol="G")], known)
        self.assertEqual(report["grown"], [])
        self.assertEqual(len(report["shrunk"]), 1)
        self.assertEqual(report["added"], [])

    def test_baseline_without_values_is_treated_as_zero(self):
        known = {ac.key_of(self.violation(5)): {}}
        report = ac.compare([self.violation(5)], known)
        self.assertEqual(len(report["grown"]), 1)
