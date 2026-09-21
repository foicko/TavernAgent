"""Isolated release evaluation. Never contacts or modifies the user's port 8890.

Start requires TAVERNAGENT_EVAL_GATEWAY_KEY; it is passed to the meter only.
Application slots receive a separate derived token through environment variables.
All inference attempts pass through gemini_meter.py and its persistent ledger.
"""
import argparse
import concurrent.futures
import hashlib
import json
import os
import random
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from config_v2 import configure_slot
from eval_metrics import DEFAULT_CONSECUTIVE_K, summarize
from eval_protocol import Client, SEED_INPUTS, evaluate_sample, result_content
from gemini_meter import MODEL, utc

ROOT = Path(__file__).resolve().parents[1]


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    with temporary.open("w", encoding="utf-8") as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)


def fingerprint(binary):
    tracked = subprocess.check_output(["git", "ls-files", "-co", "--exclude-standard", "-z"], cwd=ROOT).decode().split("\0")
    files = {name: hashlib.sha256((ROOT/name).read_bytes()).hexdigest() for name in sorted(set(tracked)) if name and (ROOT/name).is_file()}
    return {"gitHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT).decode().strip(),
            "sourceSHA256": hashlib.sha256(json.dumps(files, sort_keys=True).encode()).hexdigest(),
            "binarySHA256": hashlib.sha256(binary.read_bytes()).hexdigest(), "files": files}


class Evaluation:
    def __init__(self, args):
        self.args = args
        self.directory = args.directory.resolve()
        self.directory.mkdir(parents=True, exist_ok=True)
        self.binary = args.binary.resolve()
        if args.port == 8890 or args.meter_port == 8045 or args.port == args.meter_port:
            raise ValueError("Use independent test ports")
        key = os.environ.get("TAVERNAGENT_EVAL_GATEWAY_KEY", "")
        if not key:
            raise ValueError("Set TAVERNAGENT_EVAL_GATEWAY_KEY without printing it")
        self.gateway_key = key
        self.token = hashlib.sha256((key + "\x00TavernAgent-isolated-release-eval-v1").encode()).hexdigest()
        self.client = Client(f"http://127.0.0.1:{args.port}")

    def meter(self, path="/ledger", body=None):
        request = urllib.request.Request(f"http://127.0.0.1:{self.args.meter_port}" + path,
            data=json.dumps(body).encode() if body is not None else None,
            headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=10) as response:
            return json.load(response)

    def save_ledger(self):
        ledger = self.meter()
        write_json(self.directory/"call-ledger.json", ledger)
        return {key: ledger[key] for key in ("used", "remaining", "limit")}

    def configure(self, reflection=True):
        # 配置 v2：每个槽位指向独立实例（它们的 baseUrl 不同），密钥由 TAVERNAGENT_KEY_<SLOT> 注入。
        for slot in ("primary", "assist", "reflection"):
            configure_slot(self.client, slot, name=f"release-{slot}", kind="openai-chat",
                           base_url=f"http://127.0.0.1:{self.args.meter_port}/{slot}/v1", model=MODEL,
                           temperature=0.4, max_tokens=4096, context_window=32768,
                           enabled=reflection if slot == "reflection" else True)

    def start(self):
        env = os.environ.copy()
        env["TAVERNAGENT_EVAL_TOKEN"] = self.token
        flags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
        runtime = {}
        try:
            self.meter()
        except (OSError, ValueError):
            log = (self.directory/"meter.log").open("ab", buffering=0)
            process = subprocess.Popen([sys.executable, "-u", str(ROOT/"scripts/gemini_meter.py"), "--directory", str(self.directory/"meter"),
                "--upstream", self.args.upstream, "--port", str(self.args.meter_port)], cwd=ROOT, env=env, stdout=log, stderr=log, creationflags=flags)
            runtime["meterPID"] = process.pid
        # The application never receives the upstream credential.
        env.pop("TAVERNAGENT_EVAL_GATEWAY_KEY", None)
        for slot in ("PRIMARY", "ASSIST", "REFLECTION"):
            env["TAVERNAGENT_KEY_"+slot] = self.token
        try:
            self.client.must("GET", "/api/status")
        except (OSError, ValueError, RuntimeError):
            log = (self.directory/"application.log").open("ab", buffering=0)
            process = subprocess.Popen([str(self.binary), "-addr", f"127.0.0.1:{self.args.port}", "-data", str(self.directory/"data"), "-pin", "482619"],
                cwd=ROOT, env=env, stdout=log, stderr=log, creationflags=flags)
            runtime["applicationPID"] = process.pid
        previous = self.directory/"runtime.json"
        if previous.exists():
            runtime = {**json.loads(previous.read_text()), **runtime}
        write_json(previous, runtime)
        deadline = time.monotonic()+25
        while True:
            try:
                self.meter()
                self.client.must("GET", "/api/status")
                break
            except (OSError, ValueError, RuntimeError):
                if time.monotonic()>deadline:
                    raise RuntimeError("Isolated services did not start; see application.log and meter.log")
                time.sleep(0.25)
        self.configure()
        # 提示词版本与消融开关随指纹一起落盘（ADS-7.8-04 / 7.8-01）：
        # 报告必须能自证"这一轮跑的是哪份提示词、关了哪些特性"，
        # 否则成功率变化无法归因，基线组的可比性也无从核对。
        diagnostic = {}
        try:
            status = self.client.must("GET", "/api/status")
            diagnostic = {key: status[key] for key in ("prompt", "ablation") if key in status}
        except (OSError, ValueError, RuntimeError) as error:
            diagnostic = {"diagnosticError": str(error)}
        write_json(self.directory/"fingerprint.json", {**fingerprint(self.binary), **diagnostic})
        print(json.dumps({"started": True, "port": self.args.port, **self.save_ledger()}), flush=True)

    def guard_budget(self):
        """预检之前先看预算余额：网关的额度是**不可逆**的，烧完就只能换目录重来。

        preflight 本身也要花掉几次调用（三个槽位各探一次），所以这道闸必须放在它前面。
        需要多少：samples 次 + 一次有界修复的余量（只重跑失败样本，按 10% 且至少 10 次估）。
        """
        ledger = self.meter()
        repair = max(10, self.args.samples // 10)
        needed = self.args.samples + repair
        if ledger.get("remaining", 0) < needed:
            raise SystemExit(
                f"评测预算不足：剩余 {ledger.get('remaining', 0)} 次，至少需要 {needed} 次"
                f"（{self.args.samples} 例 + 修复余量 {repair}）。"
                "额度不可逆，请先确认网关额度，或用更小的 --samples 跑一轮探路。")
        print(json.dumps({"budget": {"remaining": ledger.get("remaining", 0), "needed": needed,
                                     "samples": self.args.samples, "repairReserve": repair}}), flush=True)

    def preflight(self):
        self.guard_budget()
        self.meter("/control/phase", {"phase": "preflight"})
        self.configure()
        results = []
        for slot in ("primary", "assist", "reflection"):
            result = self.client.must("POST", "/api/v1/config/provider/probe", {"slot": slot, "format": True})
            results.append(result)
            write_json(self.directory/"preflight.json", {"generatedAt": utc(), "results": results, **self.save_ledger()})
            print(json.dumps(result, ensure_ascii=False), flush=True)

    def protocol(self):
        report_path = self.directory/"protocol.json"
        if report_path.exists():
            raise RuntimeError("Protocol report exists; review it before allocating a separate retest")
        self.meter("/control/phase", {"phase": "protocol"})
        # Background work has its own long-story allocation. Slots retain Gemini.
        self.configure(reflection=False)
        card = json.loads((ROOT/"scripts/fixtures/m4_protocol_card.json").read_text(encoding="utf-8-sig"))
        created = self.client.must("POST", "/api/v1/sessions", {"title": "Gemini 100 independent protocol samples",
            "characterJson": json.dumps(card, ensure_ascii=False), "playerName": "旅人", "playerRole": "灯塔档案的核对者"})
        session = self.client.must("GET", "/api/v1/sessions/"+created["sessionId"])
        inputs = list(SEED_INPUTS)
        random.Random(self.args.seed).shuffle(inputs)
        report = {"generatedAt": utc(), "model": MODEL, "fingerprint": fingerprint(self.binary), "sessionId": session["sessionId"],
                  "seed": self.args.seed,
                  "note": "Independent branches from a common opening; background inference disabled; at most one continuation repair per sample.", "results": []}
        with concurrent.futures.ThreadPoolExecutor(max_workers=self.args.workers) as workers:
            futures = {workers.submit(evaluate_sample, self.client, session, inputs[i%len(inputs)], i+1, 150, 1): i for i in range(self.args.samples)}
            for future in concurrent.futures.as_completed(futures):
                result = future.result()
                report["results"].append(result)
                report["results"].sort(key=lambda row: row["sample"])
                count = len(report["results"])
                summary = summarize(report["results"], self.args.consecutive_k)
                report["summary"] = summary
                report.update(summary)
                report["ledger"] = self.save_ledger()
                # 通过门槛保持原有口径（100 例、首过 ≥95、含一次修复 ≥99）；
                # Pass^k 是同时给出的**可靠性读数**，不是新的门槛——先看到数字，
                # 再由人决定是否收紧门槛，避免"悄悄换了验收标准"。
                report["passed"] = count == 100 and summary["firstPassSuccesses"] >= 95 and summary["afterRepairSuccesses"] >= 99
                write_json(report_path, report)
                print(json.dumps({"completed": count, "sample": result["sample"], "status": result["status"],
                                  "first": summary["firstPassSuccesses"], "repaired": summary["repairedSuccesses"],
                                  "passPowerK": round(summary["passPowerK"], 4), **report["ledger"]}), flush=True)
        if not report["passed"]:
            raise SystemExit(1)

    def protocol_repair(self):
        """One bounded retry for original failures that used no continuation.

        Keep the original 100-sample report immutable. A rejected operation or
        cancelled timeout is retried from its unchanged original branch head.
        This is a separately reported repair, never a new first-pass success.
        """
        original_path = self.directory/"protocol.json"
        original = json.loads(original_path.read_text(encoding="utf-8"))
        if original["samples"] != 100:
            raise RuntimeError("Finish the original 100 samples before repair")
        path = self.directory/"protocol-one-repair.json"
        if path.exists():
            raise RuntimeError("Repair evidence already exists; do not grant a second repair")
        self.meter("/control/phase", {"phase": "retest"})
        self.configure(reflection=False)
        report = {"generatedAt": utc(), "model": MODEL, "fingerprint": fingerprint(self.binary),
                  "originalReportSHA256": hashlib.sha256(original_path.read_bytes()).hexdigest(),
                  "firstPassSuccesses": original["firstPassSuccesses"], "samples": 100,
                  "originalAfterRepairSuccesses": original["afterRepairSuccesses"],
                  "repairs": [], "completed": False, "passed": False}
        write_json(path, report)
        sid = original["sessionId"]
        for sample in original["results"]:
            if sample["firstPass"] or sample["repaired"] or sample["repairs"] > 0:
                continue
            before = self.client.must("GET", "/api/v1/turns/"+sample["turnId"])
            view = self.client.must("GET", f"/api/v1/sessions/{sid}?branchId={sample['branchId']}")
            if view["branch"]["headNodeId"] != before["expectedHeadId"] or before["status"] not in ("failed", "cancelled", "conflicted"):
                raise RuntimeError("Repair requires a terminal original request and unchanged head")
            accepted = self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{sample['branchId']}/turns", {
                "idempotencyKey": f"eval-one-repair-{sample['sample']}", "expectedHeadId": before["expectedHeadId"],
                "expectedVersion": view["branch"]["version"], "expectedCharacterId": view["characterId"],
                "mode": "structured", "input": {"kind": "text", "text": sample["input"]}})
            turn = self.client.wait_turn(accepted["turnId"], 150)
            content = result_content(turn)
            success = turn.get("status") == "committed" and content.get("provenance", {}).get("mode", turn.get("mode")) == "structured" and bool(content.get("blocks"))
            result = {"sample": sample["sample"], "originalTurnId": sample["turnId"], "strategy": "one fresh retry from unchanged head",
                      "repairs": 1, "success": success, "turn": turn}
            result["cleanupStatus"] = self.client.cancel_unfinished(accepted["turnId"], view["characterId"])
            report["repairs"].append(result)
            report["ledger"] = self.save_ledger()
            write_json(path, report)
            print(json.dumps({"sample": sample["sample"], "repairPassed": success, **report["ledger"]}), flush=True)
        after = original["afterRepairSuccesses"] + sum(row["success"] for row in report["repairs"])
        report.update(completed=True, afterRepairSuccesses=after, afterRepairRate=after/100,
                      firstPassRate=original["firstPassSuccesses"]/100,
                      passed=original["firstPassSuccesses"] >= 95 and after >= 99)
        write_json(path, report)
        if not report["passed"]:
            raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["start", "preflight", "protocol", "protocol-repair", "ledger", "configure", "features", "long-story", "long-story-tail", "compatibility"])
    parser.add_argument("--directory", type=Path, default=ROOT/"output/release-readiness/gemini")
    parser.add_argument("--binary", type=Path, default=ROOT/"build/release-eval/tavernagent.exe")
    parser.add_argument("--port", type=int, default=18890)
    parser.add_argument("--meter-port", type=int, default=18892)
    parser.add_argument("--upstream", default="http://127.0.0.1:8045/v1")
    parser.add_argument("--samples", type=int, default=100)
    parser.add_argument("--workers", type=int, default=2)
    parser.add_argument("--label", default="", help="Distinct evidence suffix for a retest; never resets the meter")
    parser.add_argument("--seed", type=int, default=20260913, help="Input-shuffle seed; vary it across 3-5 runs to see run-to-run spread")
    parser.add_argument("--consecutive-k", type=int, default=DEFAULT_CONSECUTIVE_K, help="Window for the Pass^k reliability figure (default 5)")
    args = parser.parse_args()
    evaluation = Evaluation(args)
    if args.command == "ledger":
        print(json.dumps(evaluation.save_ledger()))
    elif args.command == "configure":
        evaluation.configure()
    elif args.command in ("features", "long-story", "long-story-tail"):
        from release_features import run
        run(evaluation, args.command)
    elif args.command == "compatibility":
        from release_features import compatibility
        compatibility(evaluation)
    else:
        getattr(evaluation, args.command.replace("-", "_"))()


if __name__ == "__main__":
    main()
