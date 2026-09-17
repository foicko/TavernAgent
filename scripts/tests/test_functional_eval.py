"""功能测试脚本的纯函数回归：报告契约、日志格式、供应商解析。

不发起任何网络请求、不消耗模型额度——那些属于 functional_eval.py 的运行期职责。
"""
import io
import json
import sys
import tempfile
import unittest
from argparse import Namespace
from contextlib import redirect_stdout
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import functional_eval as fe


class DescribeSlotsTests(unittest.TestCase):
    def test_renders_slots_and_flags_missing_keys(self):
        config = {"providers": [
            {"slot": "primary", "enabled": True, "model": "gemini-x", "hasApiKey": True},
            {"slot": "reflection", "enabled": False, "kind": "openai-chat", "hasApiKey": False},
        ]}
        text = fe.describe_slots(config)
        self.assertIn("primary（启用，gemini-x，密钥已注入）", text)
        self.assertIn("reflection（停用，openai-chat，密钥缺失）", text)

    def test_handles_map_shaped_and_empty_payloads(self):
        self.assertEqual(fe.describe_slots({}), "无")
        self.assertEqual(fe.describe_slots({"providers": {"primary": {"slot": "primary", "enabled": True, "model": "m", "hasApiKey": True}}}),
                         "primary（启用，m，密钥已注入）")


class RecordFailureTests(unittest.TestCase):
    def test_reads_camel_and_snake_case_fields(self):
        self.assertEqual(fe.record_failure({"failureCode": "PROTOCOL_INVALID", "failureMessage": "unknown frame"}),
                         "PROTOCOL_INVALID: unknown frame")
        self.assertEqual(fe.record_failure({"failure_code": "X", "failure_message": "y"}), "X: y")

    def test_missing_fields_do_not_crash(self):
        self.assertEqual(fe.record_failure({}), "UNKNOWN")


class RecorderTests(unittest.TestCase):
    def test_log_carries_the_full_conversation_and_jsonl_is_machine_readable(self):
        with tempfile.TemporaryDirectory() as directory:
            recorder = fe.Recorder(Path(directory), ["标题", "模型：m"])
            recorder.rule("场景：基础对话")
            recorder.verdict("回合提交成功", True, "状态 committed")
            recorder.record_turn({
                "index": 1, "label": "动作输入", "sessionId": "s", "turnId": "t", "character": "测试角色",
                "text": "我踏上跳板。", "status": "committed", "elapsed": 1.5, "repairs": 0,
                "blocks": [{"kind": "narration", "text": "雾很重。"}, {"kind": "dialogue", "text": "上船。"}],
                "prose": "雾很重。\n上船。",
            })
            recorder.close()

            log = (Path(directory) / "conversation.log").read_text(encoding="utf-8")
            # 人读日志：标题、玩家输入、按块类型渲染的回复、判定都要在
            self.assertIn("标题", log)
            self.assertIn("[第 1 轮 · 玩家]", log)
            self.assertIn("我踏上跳板。", log)
            self.assertIn("*雾很重。*", log)
            self.assertIn("「上船。」", log)
            self.assertIn("[判定] PASS · 回合提交成功 · 状态 committed", log)

            lines = (Path(directory) / "conversation.jsonl").read_text(encoding="utf-8").strip().split("\n")
            self.assertEqual(len(lines), 1)
            self.assertEqual(json.loads(lines[0])["label"], "动作输入")


class ScenarioVerdictTests(unittest.TestCase):
    def test_warning_steps_do_not_fail_the_scenario(self):
        with tempfile.TemporaryDirectory() as directory:
            recorder = fe.Recorder(Path(directory), ["t"])
            scenario = fe.Scenario("basic", "基础对话")
            scenario.warn(recorder, "首答失败（failed）", "PROTOCOL_INVALID")
            scenario.check(recorder, "重试后成功", True)
            self.assertTrue(scenario.passed)
            self.assertTrue(scenario.steps[0]["warning"])
            recorder.close()

    def test_any_failed_step_fails_the_scenario(self):
        with tempfile.TemporaryDirectory() as directory:
            recorder = fe.Recorder(Path(directory), ["t"])
            scenario = fe.Scenario("basic", "基础对话")
            scenario.check(recorder, "ok", True)
            scenario.check(recorder, "bad", False, "证据")
            self.assertFalse(scenario.passed)
            recorder.close()


class ProviderSelectionTests(unittest.TestCase):
    def args(self, **overrides) -> Namespace:
        base = dict(provider="configured", base_url="", model="", key_env="", kind="", skip_preflight=True)
        base.update(overrides)
        return Namespace(**base)

    def test_gemini_requires_an_explicit_key_env(self):
        import os
        previous = os.environ.pop("TAVERNAGENT_EVAL_GATEWAY_KEY", None)
        try:
            with self.assertRaises(SystemExit) as failure:
                fe.resolve_provider(self.args(provider="gemini"))
            self.assertIn("TAVERNAGENT_EVAL_GATEWAY_KEY", str(failure.exception))
        finally:
            if previous is not None:
                os.environ["TAVERNAGENT_EVAL_GATEWAY_KEY"] = previous

    def test_gemini_defaults_to_the_local_gateway(self):
        import os
        os.environ["TAVERNAGENT_EVAL_GATEWAY_KEY"] = "test-token"
        try:
            provider = fe.resolve_provider(self.args(provider="gemini", model="gemini-3.5-flash-lite"))
            self.assertEqual(provider.base_url, "http://127.0.0.1:8045/v1")
            self.assertEqual(provider.model, "gemini-3.5-flash-lite")
            self.assertEqual(provider.key, "test-token")
        finally:
            os.environ.pop("TAVERNAGENT_EVAL_GATEWAY_KEY", None)

    def test_configured_reads_the_app_settings_and_secrets(self):
        provider = fe.resolve_provider(self.args())
        self.assertTrue(provider.base_url, "应能从 data/config/settings.json 读出 baseUrl")
        self.assertTrue(provider.model)
        self.assertEqual(provider.label, "configured")

    def test_overrides_win(self):
        provider = fe.resolve_provider(self.args(base_url="http://127.0.0.1:9999/v1", model="custom-model", kind="openai-responses"))
        self.assertEqual(provider.base_url, "http://127.0.0.1:9999/v1")
        self.assertEqual(provider.model, "custom-model")
        self.assertEqual(provider.kind, "openai-responses")


class BuiltinCardFixtureTests(unittest.TestCase):
    """脚本用 fixture 建会话；fixture 必须与前端的内置卡同源（防漂移）。"""

    def test_fixture_exists_and_matches_the_expected_shape(self):
        card = fe.load_json(fe.BUILTIN_CARD)
        self.assertTrue(card, f"缺少 {fe.BUILTIN_CARD}")
        self.assertEqual(card["schemaVersion"], 2)
        self.assertTrue(card["cardId"].startswith("tavernagent_original_"))
        self.assertIn("characters", card)
        # 多词字段必须是线上命名，否则角色级人设会被后端静默丢弃
        for key in ("mes_example", "system_prompt", "creator_notes"):
            self.assertIn(key, card["characters"][0], f"角色级缺少 {key}")
        self.assertTrue(card["openingVariants"])
        self.assertTrue(card["lorebookRefs"][0]["entries"])


class ServerLogWarningTests(unittest.TestCase):
    """后台任务失败只出现在服务端日志里，汇总必须稳定可读（否则报告无从解释 failed 计数）。"""

    def write(self, text: str) -> Path:
        directory = tempfile.mkdtemp()
        path = Path(directory) / "server.log"
        path.write_text(text, encoding="utf-8")
        return path

    def test_merges_identical_lines_and_orders_by_count(self):
        path = self.write(
            "2026/09/14 14:29:15 WARN summary maintenance failed branchId=b error=\"摘要 XML 含未授权层级\"\n"
            "2026/09/14 14:29:30 WARN summary maintenance failed branchId=b error=\"摘要 XML 含未授权层级\"\n"
            "2026/09/14 14:30:00 ERROR cognitive extraction failed session=s: 超时：超出总时限\n"
            "2026/09/14 14:30:01 GET /api/v1/sessions/s -> 200 (2ms)\n")
        lines = fe.server_log_warnings(path)
        self.assertEqual(lines[0], '2× WARN summary maintenance failed branchId=b error="摘要 XML 含未授权层级"')
        self.assertEqual(len(lines), 2, "普通访问日志不该进汇总")

    def test_keyword_filter_and_limit(self):
        path = self.write("2026/09/14 14:29:15 WARN summary a\n2026/09/14 14:29:16 WARN reflection b\n")
        self.assertEqual(fe.server_log_warnings(path, keyword="summary"), ["1× WARN summary a"])
        self.assertEqual(len(fe.server_log_warnings(path, limit=1)), 1)

    def test_missing_file_is_not_an_error(self):
        self.assertEqual(fe.server_log_warnings(Path("definitely/missing/server.log")), [])


class MainFlowTests(unittest.TestCase):
    """入口在缺少二进制或场景名非法时必须立刻失败，而不是跑一半。"""

    def test_missing_binary_exits_with_build_hint(self):
        argv = sys.argv
        sys.argv = ["functional_eval.py", "--binary", "definitely/missing/binary"]
        try:
            with self.assertRaises(SystemExit) as failure, redirect_stdout(io.StringIO()):
                fe.main()
            self.assertIn("build.ps1", str(failure.exception))
        finally:
            sys.argv = argv

    def test_unknown_scenario_exits(self):
        argv = sys.argv
        sys.argv = ["functional_eval.py", "--scenarios", "nope"]
        try:
            with self.assertRaises(SystemExit) as failure, redirect_stdout(io.StringIO()):
                fe.main()
            self.assertIn("未知场景", str(failure.exception))
        finally:
            sys.argv = argv


if __name__ == "__main__":
    unittest.main()
