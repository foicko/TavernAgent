"""Reproducible M4 protocol evaluation against an explicitly selected server.

Example:
  python scripts/eval_protocol.py --base-url http://127.0.0.1:18890 \
      --kind mock --model-label scripted-fixture -n 20 \
      --out docs/artifacts/m4-2026-09-12/protocol-mock.json

Each sample forks the same opening, so previous successes cannot alter the
next sample. Mock reports verify plumbing, never real-model capability.
"""
import argparse
import hashlib
import json
import os
import random
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

from eval_metrics import DEFAULT_CONSECUTIVE_K, summarize, summarize_seeds

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CARD = ROOT / "scripts/fixtures/m4_protocol_card.json"
VERSION = "m4-protocol-eval.v3"
FINAL = {"committed", "failed", "cancelled", "conflicted"}
PAUSED = {"awaiting_continuation", "awaiting_approval"}
# 报告里允许出现的供应商配置字段。API Key 一律不进证据文件。
SAFE_CONFIG_KEYS = {"slot", "enabled", "kind", "model", "temperature", "maxTokens", "contextWindow"}
SEED_INPUTS = [
    "我推门走进酒馆，向值夜的向导点头致意。",
    "我把外套挂好，询问今晚的巡逻安排。",
    "我注意到桌上的地图，问起山路的标记。",
    "我说自己只是路过，想找个地方躲雨。",
    "我坐下来，点了一杯热茶。",
    "我问街上最近有没有奇怪的事。",
    "我说不必麻烦，随便就好。",
    "我起身告辞，说明天再来。",
    "我询问她对这次旅途有什么建议。",
    "我把伞递给向导，说是顺路。",
]


class Client:
    """访问被测实例的极简 HTTP 客户端。

    显式禁用代理：被测实例固定在 127.0.0.1，一旦开发环境里设了 HTTP_PROXY，
    urllib 会把本机请求也发给代理，于是每次调用都变成 502——这类失败会被记成
    evaluation_error，看起来像"模型挂了"，实际上模型根本没被调用过
    （ADS-7.8-07：看到表现下降先怀疑评测系统本身）。
    """

    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def __init__(self, base_url, token=""):
        self.base_url = base_url.rstrip("/")
        self.token = token

    def request(self, method, path, body=None):
        data = json.dumps(body, ensure_ascii=False).encode() if body is not None else None
        headers = {"Content-Type": "application/json"} if data is not None else {}
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        request = urllib.request.Request(self.base_url + path, data=data, method=method, headers=headers)
        try:
            with self.opener.open(request, timeout=30) as response:
                raw = response.read().decode("utf-8")
                return response.status, json.loads(raw) if raw else {}
        except urllib.error.HTTPError as error:
            raw = error.read().decode("utf-8", "replace")
            try:
                return error.code, json.loads(raw)
            except ValueError:
                return error.code, {"message": raw}

    def must(self, method, path, body=None):
        status, result = self.request(method, path, body)
        if not 200 <= status < 300:
            raise RuntimeError(f"HTTP {status} {path}: {result.get('message', result)}")
        return result

    def wait_turn(self, turn_id, timeout):
        deadline = time.monotonic() + timeout
        latest = {}
        while time.monotonic() < deadline:
            latest = self.must("GET", f"/api/v1/turns/{turn_id}")
            if latest.get("status") in FINAL | PAUSED:
                return latest
            time.sleep(0.1)
        return {**latest, "evaluationTimeout": True}

    def cancel_unfinished(self, turn_id, character_id):
        current = self.must("GET", f"/api/v1/turns/{turn_id}")
        if current.get("status") not in FINAL:
            self.must("POST", f"/api/v1/turns/{turn_id}/cancel", {"expectedCharacterId": character_id})
            current = self.wait_turn(turn_id, 10)
        if current.get("status") not in FINAL:
            raise RuntimeError("cleanup could not release turn " + turn_id)
        return current.get("status")


def result_content(turn):
    node = turn.get("resultNode")
    if not node:
        return {}
    return json.loads(node["contentJson"])


def evaluate_sample(client, session, text, number, timeout, max_repairs):
    sid, cid = session["sessionId"], session["characterId"]
    branch = client.must("POST", f"/api/v1/sessions/{sid}/branches", {
        "fromNodeId": session["rootNodeId"], "name": f"sample-{number}",
        "expectedCharacterId": cid,
    })["branch"]
    result = {"sample": number, "input": text, "firstPass": False, "repaired": False,
              "branchId": branch["branchId"], "repairs": 0}
    started = time.monotonic()
    turn_id = None
    try:
        accepted = client.must("POST", f"/api/v1/sessions/{sid}/branches/{branch['branchId']}/turns", {
            "idempotencyKey": f"eval-{number}", "expectedHeadId": branch["headNodeId"],
            "expectedVersion": branch["version"], "expectedCharacterId": cid,
            "mode": "structured", "input": {"kind": "text", "text": text},
        })
        turn_id = accepted["turnId"]
        result["turnId"] = turn_id
        turn = client.wait_turn(turn_id, timeout)
        result["initialStatus"] = turn.get("status")
        result["initialFailureCode"] = turn.get("failureCode", "")
        content = result_content(turn)
        mode = content.get("provenance", {}).get("mode", turn.get("mode", ""))
        result["firstPass"] = turn.get("status") == "committed" and mode == "structured"
        while turn.get("status") == "awaiting_continuation" and result["repairs"] < max_repairs:
            result["repairs"] += 1
            client.must("POST", f"/api/v1/turns/{turn_id}/continue", {"expectedCharacterId": cid})
            turn = client.wait_turn(turn_id, timeout)
        content = result_content(turn)
        mode = content.get("provenance", {}).get("mode", turn.get("mode", ""))
        result.update(status=turn.get("status", "unknown"), mode=mode,
                      failureCode=turn.get("failureCode", ""), blocks=len(content.get("blocks", [])),
                      options=len(content.get("options", [])), timedOut=turn.get("evaluationTimeout", False))
        result["repaired"] = not result["firstPass"] and result["repairs"] > 0 and result["status"] == "committed" and mode == "structured"
    except (OSError, ValueError, RuntimeError) as error:
        result.update(status="evaluation_error", error=str(error))
    finally:
        if turn_id:
            try:
                result["cleanupStatus"] = client.cancel_unfinished(turn_id, cid)
            except (OSError, ValueError, RuntimeError) as error:
                result["cleanupError"] = str(error)
        result["elapsedMs"] = round((time.monotonic() - started) * 1000, 1)
    return result


def build_report(args, native, card_text, client, session_id, seed):
    return {
        "generatedAt": datetime.now(timezone.utc).isoformat(), "evaluationVersion": VERSION,
        "kind": args.kind, "modelLabel": args.model_label, "seed": seed,
        "cardName": native.get("name"), "cardSHA256": hashlib.sha256(card_text.encode()).hexdigest(),
        "compilerSHA256": hashlib.sha256((ROOT / "internal/context/compile.go").read_bytes()).hexdigest(),
        "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "providerConfigs": [{key: value for key, value in cfg.items() if key in SAFE_CONFIG_KEYS}
                            for cfg in client.must("GET", "/api/v1/config/provider").get("providers", [])],
        "baseUrl": args.base_url, "sessionId": session_id,
        "note": "Each sample forks the same root. First pass excludes continuation; repair is reported separately. "
                "Mock runs are deterministic integration checks, not model quality measurements.",
        "results": [],
    }


def run_seed(args, native, card_text, inputs, seed, out_path):
    """跑完一个随机种子下的全部样本，返回报告字典。

    种子只影响"哪条输入分给哪个样本"（同一批输入的重排），因此多个种子衡量的是
    采样顺序带来的波动，而不是任务分布的变化——这足以暴露"结果其实靠运气"这种情况
    （ADS-7.6-02 要求同一配置用 3–5 个种子并报告波动）。
    """
    shuffled = list(inputs)
    if not args.inputs:
        random.Random(seed).shuffle(shuffled)
    client = Client(args.base_url, os.environ.get("TAVERNAGENT_AUTH_TOKEN", ""))
    created = client.must("POST", "/api/v1/sessions", {
        "title": f"M4 协议评测 {args.kind} seed={seed}",
        "characterJson": card_text, "playerName": "旅人", "playerRole": "来访的探险者",
    })
    session = client.must("GET", "/api/v1/sessions/" + created["sessionId"])
    report = build_report(args, native, card_text, client, created["sessionId"], seed)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    for index in range(args.samples):
        sample = evaluate_sample(client, session, shuffled[index % len(shuffled)], index + 1, args.timeout, args.max_repairs)
        report["results"].append(sample)
        summary = summarize(report["results"], args.consecutive_k)
        report["summary"] = summary
        # 顶层同时铺开一份，保持既有证据文件读取方的兼容（firstPassRate 等字段名不变）。
        report.update(summary)
        out_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"seed={seed} {index+1}/{args.samples}: {sample['status']} first={sample['firstPass']} repairs={sample['repairs']}", flush=True)
        if sample.get("cleanupError"):
            raise SystemExit("Cleanup failed; stopped to avoid leaving additional active turns. See report.")
    print(f"seed={seed} {args.kind}: first {summary['firstPassSuccesses']}/{summary['samples']}; "
          f"after repair {summary['afterRepairSuccesses']}/{summary['samples']}; "
          f"Pass^{args.consecutive_k}={summary['passPowerK']:.3f}; report {out_path}")
    if any(item.get("status") == "evaluation_error" for item in report["results"]):
        raise SystemExit(1)
    return report


def parse_seeds(raw, fallback):
    if not raw:
        return [fallback]
    seeds = []
    for part in raw.split(","):
        part = part.strip()
        if not part:
            continue
        try:
            seeds.append(int(part))
        except ValueError:
            raise SystemExit(f"--seeds 只接受整数，得到 {part!r}")
    if not seeds:
        raise SystemExit("--seeds 为空；留空表示沿用 --seed")
    return seeds


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="Explicit test instance URL")
    parser.add_argument("--kind", choices=["mock", "real"], required=True)
    parser.add_argument("--model-label", required=True, help="Model and version under test")
    parser.add_argument("--card", type=Path, default=DEFAULT_CARD, help="Native JSON character fixture")
    parser.add_argument("-n", "--samples", type=int, default=20)
    parser.add_argument("-seed", "--seed", type=int, default=20260912)
    parser.add_argument("--seeds", default="", help="逗号分隔的多个随机种子（3–5 个）；给出后 --out 写聚合结果，单种子报告写入同目录 <out-stem>.seed<N>.json")
    parser.add_argument("--consecutive-k", type=int, default=DEFAULT_CONSECUTIVE_K,
                        help="Pass^k 的连续轮数口径（默认 5）")
    parser.add_argument("--timeout", type=float, default=60)
    parser.add_argument("--max-repairs", type=int, default=1)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--inputs", type=Path, help="Optional JSON array of scripted inputs")
    args = parser.parse_args()
    if args.samples < 1 or args.timeout <= 0 or args.max_repairs < 0:
        parser.error("samples/timeout must be positive and max-repairs nonnegative")
    if args.consecutive_k < 1:
        parser.error("consecutive-k must be positive")
    native = json.loads(args.card.read_text(encoding="utf-8-sig"))
    card_text = json.dumps(native, ensure_ascii=False, sort_keys=True)
    inputs = json.loads(args.inputs.read_text(encoding="utf-8-sig")) if args.inputs else list(SEED_INPUTS)
    if not inputs or not all(isinstance(text, str) and text.strip() for text in inputs):
        parser.error("inputs must be a nonempty array of strings")

    seeds = parse_seeds(args.seeds, args.seed)
    reports = []
    for seed in seeds:
        target = args.out if len(seeds) == 1 else args.out.with_name(f"{args.out.stem}.seed{seed}{args.out.suffix}")
        reports.append(run_seed(args, native, card_text, inputs, seed, target))
    if len(seeds) == 1:
        return
    # 多种子：--out 写聚合。单种子报告保持独立文件，便于逐份复核。
    aggregate = {
        "generatedAt": datetime.now(timezone.utc).isoformat(), "evaluationVersion": VERSION,
        "kind": "seed-aggregate", "modelLabel": args.model_label,
        "samplesPerSeed": args.samples, "seedReports": [str(path) for path in
            (args.out.with_name(f"{args.out.stem}.seed{seed}{args.out.suffix}") for seed in seeds)],
        "meta": summarize_seeds(reports, args.consecutive_k),
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(aggregate, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"aggregate over {len(seeds)} seeds -> {args.out}")


if __name__ == "__main__":
    main()
