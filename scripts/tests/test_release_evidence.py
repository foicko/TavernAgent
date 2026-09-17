import contextlib
import io
import json
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from release_features import compatibility


class EvidenceTests(unittest.TestCase):
    def run_suite(self, directory, responses):
        client = Mock()
        client.must.side_effect = responses
        evaluation = SimpleNamespace(directory=Path(directory), binary=Path("unused-fixture"),
            args=SimpleNamespace(label="", meter_port=18892), client=client,
            configure=Mock(), meter=Mock(), save_ledger=lambda: {"used": 0, "remaining": 300, "limit": 300})
        with patch("release_eval.fingerprint", return_value={"binarySHA256": "fixture"}), contextlib.redirect_stdout(io.StringIO()):
            compatibility(evaluation)

    def test_interrupted_second_protocol_cannot_publish_passing_report(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                self.run_suite(directory, [{}, {"ok": True}, RuntimeError("second protocol interrupted")])
            report = json.loads((Path(directory)/"compatibility.json").read_text(encoding="utf-8"))
            self.assertFalse(report["passed"])
            self.assertFalse(report.get("completed", False))
            self.assertTrue(report["cases"][0]["passed"])

    def test_failed_probe_returns_failure_exit_status(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(SystemExit) as failure:
                self.run_suite(directory, [{}, {"ok": True}, {}, {"ok": False}])
            self.assertEqual(failure.exception.code, 1)
            report = json.loads((Path(directory)/"compatibility.json").read_text(encoding="utf-8"))
            self.assertTrue(report["completed"])
            self.assertFalse(report["passed"])


if __name__ == "__main__":
    unittest.main()
