import sys
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from gemini_meter import BudgetExhausted, Ledger


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


if __name__ == "__main__":
    unittest.main()
