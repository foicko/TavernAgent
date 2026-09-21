import os
import sys
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from gemini_meter import BudgetExhausted, Ledger, MODEL as DEFAULT_MODEL


class BudgetTests(unittest.TestCase):
    def test_concurrent_reservations_restart_and_failures_count(self):
        with tempfile.TemporaryDirectory() as directory:
            ledger = Ledger(directory, 17)
            def reserve(_):
                try:
                    return ledger.reserve("primary", "chat/completions", b"{}")
                except BudgetExhausted:
                    return None
            with ThreadPoolExecutor(max_workers=12) as workers:
                accepted = [i for i in workers.map(reserve, range(55)) if i is not None]
            self.assertEqual(len(accepted), 17)
            ledger.finish(accepted[0], "upstream_error", 500, 1, None, 0, "")
            resumed = Ledger(directory, 17)
            self.assertEqual(resumed.snapshot()["remaining"], 0)
            with self.assertRaises(BudgetExhausted):
                resumed.reserve("reflection", "chat/completions", b"{}")
            with self.assertRaises(ValueError):
                Ledger(directory, 18)
            self.assertEqual(resumed.snapshot()["used"], 17)

    def test_phase_is_persisted_with_each_attempt(self):
        with tempfile.TemporaryDirectory() as directory:
            ledger = Ledger(directory, 2)
            ledger.phase("long_story")
            ledger.reserve("reflection", "chat/completions", b"{}")
            self.assertEqual(Ledger(directory, 2).snapshot()["calls"][0]["phase"], "long_story")

    def test_model_label_is_recorded_and_configurable(self):
        # 模型标签可以换（--model / TAVERNAGENT_EVAL_MODEL），但必须逐次记进台账：
        # 否则"这一轮到底跑了哪个模型"只能靠考古，发布证据也就无法对账。
        with tempfile.TemporaryDirectory() as directory:
            ledger = Ledger(directory, 2, "another-model")
            ledger.reserve("primary", "chat/completions", b"{}")
            snapshot = Ledger(directory, 2, "another-model").snapshot()
            self.assertEqual(snapshot["model"], "another-model")
            self.assertEqual(snapshot["calls"][0]["model"], "another-model")

    def test_default_model_label_is_unchanged(self):
        # 默认值一改，历史台账与新报告的标签就会分叉，所以这里把它钉住。
        # 显式设了环境变量时，断言"覆盖确实生效"，而不是硬编码默认值。
        override = os.environ.get("TAVERNAGENT_EVAL_MODEL")
        self.assertEqual(DEFAULT_MODEL, override or "gemini-3.8-flash-high")


if __name__ == "__main__":
    unittest.main()
