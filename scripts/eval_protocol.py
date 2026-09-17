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

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CARD = ROOT / "scripts/fixtures/m4_protocol_card.json"
VERSION = "m4-protocol-eval.v2"
FINAL = {"committed", "failed", "cancelled", "conflicted"}
PAUSED = {"awaiting_continuation", "awaiting_approval"}
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
            with urllib.request.urlopen(request, timeout=30) as response:
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="Explicit test instance URL")
    parser.add_argument("--kind", choices=["mock", "real"], required=True)
    parser.add_argument("--model-label", required=True, help="Model and version under test")
    parser.add_argument("--card", type=Path, default=DEFAULT_CARD, help="Native JSON character fixture")
    parser.add_argument("-n", "--samples", type=int, default=20)
    parser.add_argument("-seed", "--seed", type=int, default=20260912)
    parser.add_argument("--timeout", type=float, default=60)
    parser.add_argument("--max-repairs", type=int, default=1)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--inputs", type=Path, help="Optional JSON array of scripted inputs")
    args = parser.parse_args()
    if args.samples < 1 or args.timeout <= 0 or args.max_repairs < 0:
        parser.error("samples/timeout must be positive and max-repairs nonnegative")
    native = json.loads(args.card.read_text(encoding="utf-8-sig"))
    card_text = json.dumps(native, ensure_ascii=False, sort_keys=True)
    inputs = json.loads(args.inputs.read_text(encoding="utf-8-sig")) if args.inputs else list(SEED_INPUTS)
    if not inputs or not all(isinstance(text, str) and text.strip() for text in inputs):
        parser.error("inputs must be a nonempty array of strings")
    if not args.inputs:
        random.Random(args.seed).shuffle(inputs)
    client = Client(args.base_url, os.environ.get("TAVERNAGENT_AUTH_TOKEN", ""))
    created = client.must("POST", "/api/v1/sessions", {
        "title": f"M4 协议评测 {args.kind} seed={args.seed}",
        "characterJson": card_text, "playerName": "旅人", "playerRole": "来访的探险者",
    })
    session = client.must("GET", "/api/v1/sessions/" + created["sessionId"])
    configs = client.must("GET", "/api/v1/config/provider").get("providers", [])
    safe_keys = {"slot", "enabled", "kind", "model", "temperature", "maxTokens", "contextWindow"}
    report = {
        "generatedAt": datetime.now(timezone.utc).isoformat(), "evaluationVersion": VERSION,
        "kind": args.kind, "modelLabel": args.model_label, "seed": args.seed,
        "cardName": native.get("name"), "cardSHA256": hashlib.sha256(card_text.encode()).hexdigest(),
        "compilerSHA256": hashlib.sha256((ROOT / "internal/context/compile.go").read_bytes()).hexdigest(),
        "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "providerConfigs": [{key: value for key, value in cfg.items() if key in safe_keys} for cfg in configs],
        "baseUrl": args.base_url, "sessionId": created["sessionId"],
        "note": "Each sample forks the same root. First pass excludes continuation; repair is reported separately. Mock runs are deterministic integration checks, not model quality measurements.",
        "results": [],
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    for index in range(args.samples):
        sample = evaluate_sample(client, session, inputs[index % len(inputs)], index + 1, args.timeout, args.max_repairs)
        report["results"].append(sample)
        first = sum(item["firstPass"] for item in report["results"])
        repaired = sum(item["repaired"] for item in report["results"])
        count = len(report["results"])
        report.update(samples=count, firstPassSuccesses=first, repairedSuccesses=repaired,
                      firstPassRate=first/count, afterRepairRate=(first+repaired)/count)
        args.out.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"{index+1}/{args.samples}: {sample['status']} first={sample['firstPass']} repairs={sample['repairs']}", flush=True)
        if sample.get("cleanupError"):
            raise SystemExit("Cleanup failed; stopped to avoid leaving additional active turns. See report.")
    print(f"{args.kind}: first {first}/{count}; after repair {first+repaired}/{count}; report {args.out}")
    if any(item.get("status") == "evaluation_error" for item in report["results"]):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
