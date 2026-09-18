"""eval_metrics 的口径与归因单测。

为什么值得单测：Pass@k / Pass^k 是**给人做决策的数字**。算错一个指数不会报错，
只会让报告给出乐观得多的结论——这类"静默错误"必须靠断言钉住。
"""
import sys
import unittest
from pathlib import Path

# 与 scripts/tests 下其它用例一致：把 scripts/ 加进 sys.path，
# 否则单独运行本文件时会 ModuleNotFoundError（依赖其它用例先插入路径是不可靠的）。
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from eval_metrics import (  # noqa: E402
    DEFAULT_CONSECUTIVE_K,
    attribute_failure,
    classify_failure,
    failure_clusters,
    pass_at_k,
    pass_power_k,
    standard_error,
    summarize,
    summarize_seeds,
)


def sample(number, first_pass=False, repaired=False, **extra):
    row = {"sample": number, "firstPass": first_pass, "repaired": repaired, "repairs": 0}
    row.update(extra)
    return row


class PassMetricTest(unittest.TestCase):
    def test_known_values_from_spec(self):
        # 规范里的示例：p=0.6, k=5 → Pass@5 ≈ 99.0%，Pass^5 ≈ 7.8%。
        self.assertAlmostEqual(pass_at_k(0.6, 5), 0.98976, places=5)
        self.assertAlmostEqual(pass_power_k(0.6, 5), 0.07776, places=5)

    def test_message_quality_gap_is_visible(self):
        # 100 例里 95 次首过听起来很好；连续 5 轮全部成功的概率只有约 77%。
        self.assertAlmostEqual(pass_power_k(0.95, 5), 0.7737809375, places=6)
        self.assertGreater(pass_at_k(0.95, 5), 0.9999)

    def test_rejects_non_positive_k(self):
        for func in (pass_at_k, pass_power_k):
            with self.assertRaises(ValueError):
                func(0.9, 0)


class ClassifyTest(unittest.TestCase):
    def test_evaluation_system_failure_wins_over_timeout(self):
        # 评测系统自身的错误必须单独成簇：否则模型的分数会被评测脚本的 bug 拖低。
        row = sample(1, timedOut=True, error="connection refused")
        self.assertEqual(classify_failure(row), "evaluation_system")

    def test_timeout_wins_over_protocol_violation(self):
        # 既超时又被判协议违反时，根因是超时——顺序错了会把超时算成模型不听话。
        row = sample(1, status="failed", failureCode="PROTOCOL_INVALID_FRAME", timedOut=True)
        self.assertEqual(classify_failure(row), "timeout")

    def test_provider_and_validation_classes(self):
        self.assertEqual(classify_failure(sample(1, status="failed", failureCode="PROVIDER_UNAVAILABLE")),
                         "provider_unavailable")
        self.assertEqual(classify_failure(sample(1, status="failed", failureCode="PROTOCOL_INVALID_FRAME")),
                         "protocol_violation")
        self.assertEqual(classify_failure(sample(1, status="committed", mode="compat")), "protocol_fallback")
        self.assertEqual(classify_failure(sample(1, status="cancelled")), "concurrency_conflict")

    def test_cleanup_failure_is_its_own_cluster(self):
        row = sample(1, status="failed", failureCode="PROTOCOL_INVALID_FRAME", cleanupError="release failed")
        self.assertEqual(classify_failure(row), "cleanup_failure")


class AttributeTest(unittest.TestCase):
    def test_attribution_has_required_fields_and_low_confidence(self):
        row = sample(7, status="failed", initialStatus="failed", failureCode="PROTOCOL_INVALID_FRAME",
                     mode="compat", blocks=0)
        got = attribute_failure(row)
        for key in ("taskGoal", "firstErrorStep", "errorClass", "rootCauseSide", "evidence", "confidence"):
            self.assertIn(key, got, key)
        # 由状态码推导而非人工复核，置信度必须标低——不标就等于假装已经定论。
        self.assertEqual(got["confidence"], "low")
        self.assertEqual(got["rootCauseSide"], "model")
        self.assertEqual(got["evidence"]["failureCode"], "PROTOCOL_INVALID_FRAME")


class SummarizeTest(unittest.TestCase):
    def test_keeps_legacy_fields_and_adds_both_k_metrics(self):
        results = [sample(i, first_pass=(i <= 9)) for i in range(1, 11)]  # 9/10
        got = summarize(results, consecutive_k=5)
        self.assertEqual(got["samples"], 10)
        self.assertEqual(got["firstPassSuccesses"], 9)
        self.assertAlmostEqual(got["firstPassRate"], 0.9)
        self.assertAlmostEqual(got["passAtK"], pass_at_k(0.9, 5))
        self.assertAlmostEqual(got["passPowerK"], pass_power_k(0.9, 5))
        # 口径必须写在报告里：单次观测推导 vs 逐任务重复 k 次实测，是两回事。
        self.assertIn("单次观测", got["metricBasis"])
        self.assertEqual(got["consecutiveK"], 5)

    def test_empty_results_do_not_divide_by_zero(self):
        got = summarize([])
        self.assertEqual(got["samples"], 0)
        self.assertEqual(got["firstPassRate"], 0.0)
        self.assertEqual(got["failureAttributions"], [])

    def test_clusters_only_count_unrecovered_failures(self):
        results = [sample(1, first_pass=True), sample(2, repaired=True),
                   sample(3, status="failed", failureCode="PROVIDER_UNAVAILABLE"),
                   sample(4, status="failed", failureCode="PROVIDER_UNAVAILABLE")]
        self.assertEqual(failure_clusters(results), {"provider_unavailable": 2})
        self.assertEqual(len(summarize(results)["failureAttributions"]), 2)

    def test_seed_aggregate_reports_spread(self):
        reports = [
            {"seed": 1, "summary": {"firstPassRate": 0.9}},
            {"seed": 2, "summary": {"firstPassRate": 1.0}},
            {"seed": 3, "summary": {"firstPassRate": 0.8}},
        ]
        got = summarize_seeds(reports, 5)
        self.assertEqual(got["seeds"], [1, 2, 3])
        self.assertAlmostEqual(got["firstPassRate"]["mean"], 0.9)
        self.assertGreater(got["firstPassRate"]["stdev"], 0)
        self.assertAlmostEqual(got["passPowerK"]["mean"], pass_power_k(0.9, 5))
        self.assertIn("配对分析", got["note"])

    def test_seed_aggregate_requires_input(self):
        with self.assertRaises(ValueError):
            summarize_seeds([])


class StandardErrorTest(unittest.TestCase):
    def test_matches_spec_reference(self):
        # n=100、p=0.7 → 95% 置信区间约 ±9 个百分点（1.96 × SE）。
        self.assertAlmostEqual(1.96 * standard_error(0.7, 100), 0.0898, places=3)

    def test_default_window(self):
        self.assertEqual(DEFAULT_CONSECUTIVE_K, 5)

    def test_zero_n(self):
        self.assertEqual(standard_error(0.5, 0), 0.0)


if __name__ == "__main__":
    unittest.main()
