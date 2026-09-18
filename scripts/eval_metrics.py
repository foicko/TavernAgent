"""评测指标口径与失败归因（ADS-7.2 / 7.4 / 7.6）。

为什么单独成一个模块：**Pass@k 与 Pass^k 的差别直接决定"协议能不能上线"**。
100 次独立采样里 95 次首过听起来很好，但换算成"连续 5 轮全部成功"只剩
0.95^5 ≈ 77.4%——对一场要跑几十轮的对话，这个数字才是玩家实际感受到的可靠性。
两个数字必须同时出现在报告里，否则结论会乐观得多（ADS-7.2-06）。

同时提供结构化失败归因（ADS-7.4-09）：只统计总分看不出"失败集中在哪一类"，
报告要先回答"失败簇在哪、首个错误是什么、根因在模型还是 Harness"。

口径纪律（务必随报告一起阅读）：
* Pass@k / Pass^k 在**单次观测**下由首过率 p̂ 推导，不是逐任务重复 k 次的实测值；
  要得到实测值必须用 `--seed` 跑多个种子（见 summarize_seeds）。
* 归因字段由**状态码与内容标志**推导，不读模型自述文本；因此 confidence 一律标为
  low，只用于分流与聚合，不用于单条定论。
* 表现下降时要先怀疑评测系统本身（ADS-7.8-07）：evaluation_error / 清理失败
  这类信号必须单独成簇，不能和模型失败混在一起统计。
"""

from __future__ import annotations

import statistics

# 连续成功轮数的默认口径。取 5 与规范中的示例（Pass@5 vs Pass^5）对齐。
# 这是"业务可靠性"的默认观察窗口，不是产品承诺。
DEFAULT_CONSECUTIVE_K = 5


def pass_at_k(rate: float, k: int) -> float:
    """Pass@k：k 次独立尝试里至少一次成功（能力上限）。公式 1-(1-p)^k。"""
    if k <= 0:
        raise ValueError("k must be positive")
    return 1.0 - (1.0 - rate) ** k


def pass_power_k(rate: float, k: int) -> float:
    """Pass^k：k 次连续尝试全部成功（业务可靠性）。公式 p^k。"""
    if k <= 0:
        raise ValueError("k must be positive")
    return rate**k


# 失败类别 → 根因责任方。用于把"看起来都一样"的失败分开统计。
# provider/network 归环境，protocol/validation 归模型（契约被违反），
# evaluation_* 归评测系统自身——后者的存在意义是提醒：先修尺子再看身高。
_ERROR_CLASS_SIDE = {
    "evaluation_system": "harness",
    "cleanup_failure": "harness",
    "timeout": "environment",
    "provider_unavailable": "environment",
    "process_interrupted": "environment",
    "concurrency_conflict": "environment",
    "protocol_violation": "model",
    "protocol_fallback": "model",
    "empty_output": "model",
    "unknown": "unknown",
}


def classify_failure(sample: dict) -> str:
    """把一条失败样本归入稳定的类别。

    判定顺序即优先级：先排除"评测系统自身的问题"，再看超时，再看供应商/中断，
    最后才是模型侧（协议违反、降级、空输出）。顺序很重要——一个既超时又被判
    协议违反的样本，根因是超时而不是模型不听话。
    """
    if sample.get("error") or sample.get("status") == "evaluation_error":
        return "evaluation_system"
    if sample.get("cleanupError") or sample.get("cleanupStatus") not in (None, "", "cancelled", "committed", "failed", "conflicted"):
        return "cleanup_failure"
    if sample.get("timedOut") or sample.get("evaluationTimeout"):
        return "timeout"

    code = str(sample.get("failureCode") or sample.get("initialFailureCode") or "").upper()
    status = str(sample.get("status") or "")
    if "PROVIDER" in code or "UPSTREAM" in code or "NETWORK" in code:
        return "provider_unavailable"
    if "INTERRUPT" in code:
        return "process_interrupted"
    if status in ("cancelled", "conflicted"):
        return "concurrency_conflict"
    if "VALID" in code or "PROTOCOL" in code or "FRAME" in code or "PARSE" in code:
        return "protocol_violation"
    mode = str(sample.get("mode") or "")
    if mode and mode != "structured":
        return "protocol_fallback"
    if not sample.get("blocks"):
        return "empty_output"
    return "unknown"


def attribute_failure(sample: dict) -> dict:
    """按 ADS-7.4-09 的结构化字段记录一次失败。

    字段与规范对齐：任务目标、首错步号、错误类别、根因责任方、原文证据，
    并区分根因与后果、是否可恢复、置信度。
    """
    kind = classify_failure(sample)
    # 首个出错步骤：本项目一轮只有四个可观测阶段，按"最早可能出错的位置"判定。
    if kind == "evaluation_system":
        step = "evaluate"
    elif sample.get("initialStatus") in ("failed", "cancelled", "conflicted"):
        step = "generate"
    elif sample.get("status") in ("failed", "cancelled", "conflicted"):
        step = "validate"
    elif kind == "timeout":
        step = "generate"
    else:
        step = "commit"
    return {
        "sample": sample.get("sample"),
        "taskGoal": "按逐行 JSON 帧协议完成一个结构化回合",
        "firstErrorStep": step,
        "errorClass": kind,
        "rootCauseSide": _ERROR_CLASS_SIDE.get(kind, "unknown"),
        "recoverable": kind in ("timeout", "provider_unavailable", "concurrency_conflict", "protocol_fallback"),
        "confidence": "low",
        "rootCauseNote": "由状态码与内容标志推导，未人工复核完整轨迹",
        "evidence": {
            "status": sample.get("status"),
            "initialStatus": sample.get("initialStatus"),
            "failureCode": sample.get("failureCode") or sample.get("initialFailureCode") or "",
            "mode": sample.get("mode"),
            "blocks": sample.get("blocks"),
            "repairs": sample.get("repairs"),
            "error": sample.get("error", ""),
        },
        "consequences": [k for k in ("cleanupError", "cleanupStatus") if sample.get(k)],
    }


def failure_clusters(results: list[dict]) -> dict:
    """按错误类别聚簇。只盯总分容易忽略"小而集中"的失败簇（ADS-7.8-08）。"""
    clusters: dict[str, int] = {}
    for sample in results:
        if sample.get("firstPass") or sample.get("repaired"):
            continue
        kind = classify_failure(sample)
        clusters[kind] = clusters.get(kind, 0) + 1
    return dict(sorted(clusters.items(), key=lambda item: (-item[1], item[0])))


def summarize(results: list[dict], consecutive_k: int = DEFAULT_CONSECUTIVE_K) -> dict:
    """汇总一批样本，同时给出能力上限与业务可靠性两个口径。

    返回字段是 release_eval / eval_protocol 报告的上位集合，
    原有字段（firstPassRate 等）保持兼容，便于既有证据文件继续被读取。
    """
    count = len(results)
    first = sum(1 for sample in results if sample.get("firstPass"))
    repaired = sum(1 for sample in results if sample.get("repaired"))
    after = first + repaired
    rate = first / count if count else 0.0
    return {
        "samples": count,
        "firstPassSuccesses": first,
        "repairedSuccesses": repaired,
        "afterRepairSuccesses": after,
        "firstPassRate": rate,
        "afterRepairRate": after / count if count else 0.0,
        # 能力上限：偶尔能不能成。
        "passAt1": rate,
        "passAtK": pass_at_k(rate, consecutive_k),
        # 业务可靠性：能不能一直成。协议可靠性以这个为准。
        "passPowerK": pass_power_k(rate, consecutive_k),
        "consecutiveK": consecutive_k,
        "metricBasis": (
            "Pass@k 与 Pass^k 由**单次观测**的首过率 p̂ 推导："
            f"Pass@{consecutive_k} = 1-(1-p̂)^{consecutive_k}，Pass^{consecutive_k} = p̂^{consecutive_k}；"
            "口径为同一批任务的独立采样，不是逐任务重复 k 次的实测值。"
            "要得到实测值需用 --seed 跑多个种子。"
        ),
        "failureClusters": failure_clusters(results),
        "failureAttributions": [
            attribute_failure(sample)
            for sample in results
            if not sample.get("firstPass") and not sample.get("repaired")
        ],
    }


def summarize_seeds(seed_reports: list[dict], consecutive_k: int = DEFAULT_CONSECUTIVE_K) -> dict:
    """跨种子汇总：报告均值与波动，而不是单次运行的点估计（ADS-7.6-02）。

    规范要求同一配置用 3–5 个随机种子并给出波动；单次运行只能用来筛方向。
    """
    if not seed_reports:
        raise ValueError("seed_reports must not be empty")
    rates = [report["summary"]["firstPassRate"] for report in seed_reports]
    return {
        "seeds": [report.get("seed") for report in seed_reports],
        "runs": len(seed_reports),
        "firstPassRate": {
            "mean": statistics.fmean(rates),
            "stdev": statistics.stdev(rates) if len(rates) > 1 else 0.0,
            "min": min(rates),
            "max": max(rates),
        },
        "passAtK": {
            "mean": pass_at_k(statistics.fmean(rates), consecutive_k),
            "stdev": statistics.stdev([pass_at_k(r, consecutive_k) for r in rates]) if len(rates) > 1 else 0.0,
        },
        "passPowerK": {
            "mean": pass_power_k(statistics.fmean(rates), consecutive_k),
            "stdev": statistics.stdev([pass_power_k(r, consecutive_k) for r in rates]) if len(rates) > 1 else 0.0,
        },
        "consecutiveK": consecutive_k,
        "note": (
            "多随机种子只用于判断方向与波动。切换模型或改动 Harness 需要三者齐备："
            "分差超过噪声、在配对分析中成立、能复现（ADS-7.6-02/03）。"
            f"当前 {len(seed_reports)} 个种子；规范建议 3–5 个。"
        ),
    }


def standard_error(rate: float, n: int) -> float:
    """成功率的标准误近似 sqrt(p(1-p)/n)，用于判断分差是否只是噪声（ADS-7.6-01）。

    参考量级：n=100、p=0.7 时 95% 置信区间约 ±9 个百分点——"73% vs 70%"
    不足以支持任何切换决策。
    """
    if n <= 0:
        return 0.0
    return (rate * (1.0 - rate) / n) ** 0.5
