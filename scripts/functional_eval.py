"""真实模型功能测试：按剧本驱动被测服务，产出含完整对话记录的日志。

覆盖：基础对话、多轮连贯、记忆提取与修订、上下文压缩、导演模式、
世界书、分支分叉、剧情包往返。

用法（默认使用 data/config 里已配置的供应商）：

    python scripts/functional_eval.py
    python scripts/functional_eval.py --provider gemini
    python scripts/functional_eval.py --base-url http://127.0.0.1:8045/v1 --model gemini-3.5-flash-lite --key-env TAVERNAGENT_EVAL_GATEWAY_KEY

产物（默认 output/functional/<时间戳>-<provider>/）：

    conversation.log    完整对话记录（人读：开场、每轮输入与回复、判定）
    conversation.jsonl  每回合结构化记录（机读）
    report.json         场景判定与证据
    server.log          被测服务日志

设计约束：
  * 只驱动被测服务的 HTTP 接口，回合提交字段与前端产品路径一致；
  * 测试数据写在独立的 -data 目录，绝不触碰 data/ 里的真实故事库；
  * 密钥只经环境变量传给被测进程（TAVERNAGENT_KEY_*），不落盘、不打印；
  * 被测服务用 build 产物，避免"源码能跑、产物不能跑"的假结论。
"""
import argparse
import hashlib
import json
import os
import socket
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from config_v2 import configure_slot  # noqa: E402  配置 API v2（实例 + 槽位引用）
from eval_protocol import Client, result_content  # noqa: E402  复用既有 HTTP 客户端

DEFAULT_BINARY = ROOT / "build/browser" / ("tavernagent.exe" if os.name == "nt" else "tavernagent")
BUILTIN_CARD = ROOT / "scripts/fixtures/builtin_character_card.json"
# 压缩场景把 primary 的窗口压到这么小，几轮之后就会超出保留窗口而生成摘要。
#
# 不要再往下调：窗口过小会让预算裁剪把摘要本身淘汰掉（摘要区块装不进输入预算），
# 而它覆盖的原文早已折叠移除，于是整段中期剧情静默消失、模型开始编地名——
# 那是预算饥饿，不是压缩。8192 是实测能同时触发折叠、又装得下摘要的下限：
# 本卡人设 + 状态 + 输入约 2000 token，两条摘要约 900 token，留出的余量刚好够用。
COMPACTION_WINDOW = 8192
COMPACTION_OUTPUT = 1024
# 约束窗口：小到连承重摘要都装不下，用来验证"必须显式报错"的契约。
CONSTRAINT_WINDOW = 4096
NORMAL_WINDOW = 65536
SCENARIOS = ("basic", "memory", "compaction", "director", "lorebook", "branch", "pack")


def utc() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_json(path: Path) -> dict:
    if not path.is_file():
        return {}
    return json.loads(path.read_text(encoding="utf-8-sig"))


# ---------------------------------------------------------------- 供应商


@dataclass
class Provider:
    kind: str
    base_url: str
    model: str
    key: str
    label: str
    source: str

    def describe(self) -> str:
        return f"{self.model} @ {self.base_url}（kind={self.kind}，来源：{self.source}）"


def resolve_provider(args, root: Path = ROOT) -> Provider:
    """解析被测供应商。root 可覆盖，便于测试用临时配置夹具（不依赖本机 data/）。

    配置形态是 v2：settings.json 里 models 是唯一携带连接信息的地方，
    slots.primary.modelId 指向其中一个实例；密钥存在 secrets.json 的 apiKeyRef 下。
    """
    settings = load_json(root / "data/config/settings.json")
    secrets = load_json(root / "data/config/secrets.json")
    models = settings.get("models") or {}

    if args.provider == "gemini":
        model = args.model
        if not model:
            # 本地网关（端口 8045）的实例优先，其次回退到脚本默认模型。
            for instance in models.values():
                if "8045" in str(instance.get("baseUrl", "")) and instance.get("model"):
                    model = instance["model"]
                    break
        env_name = args.key_env or "TAVERNAGENT_EVAL_GATEWAY_KEY"
        key = os.environ.get(env_name, "")
        if not key:
            raise SystemExit(
                f"未设置 {env_name}：Gemini 网关密钥只经环境变量提供。\n"
                f'  PowerShell: $env:{env_name}="<网关密钥>"\n'
                f"  然后重跑本脚本。"
            )
        return Provider(kind=args.kind or "openai-chat", base_url=args.base_url or "http://127.0.0.1:8045/v1",
                        model=model or "gemini-3.5-flash-lite", key=key, label="gemini", source=f"env {env_name}")

    primary_id = ((settings.get("slots") or {}).get("primary") or {}).get("modelId") or ""
    primary = models.get(primary_id) or {}
    env_name = args.key_env or "TAVERNAGENT_KEY_PRIMARY"
    from_env = os.environ.get(env_name, "")
    key = from_env or str(secrets.get(primary.get("apiKeyRef") or "", ""))
    if not (args.base_url or primary.get("baseUrl")):
        raise SystemExit("data/config/settings.json 里没有 primary 模型实例，请先在界面配置模型，或用 --base-url/--model/--key-env 指定。")
    return Provider(
        kind=args.kind or primary.get("kind") or "openai-chat",
        base_url=args.base_url or primary.get("baseUrl") or "",
        model=args.model or primary.get("model") or "",
        key=key,
        label="configured",
        source=f"data/config/settings.json（密钥来自 env {env_name}）" if from_env else "data/config/settings.json + secrets.json",
    )


def preflight(provider: Provider) -> None:
    """花 0 token 确认凭据可用：密钥无效立刻退出，而不是跑完剧本才发现。"""
    if not provider.base_url or not provider.model:
        raise SystemExit("供应商缺少 baseUrl 或 model。")
    request = urllib.request.Request(provider.base_url.rstrip("/") + "/models",
                                     headers={"Authorization": "Bearer " + provider.key} if provider.key else {})
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            if response.status >= 400:
                raise SystemExit(f"供应商预检失败：HTTP {response.status}")
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", "replace")[:160]
        raise SystemExit(
            f"供应商预检失败：HTTP {error.code} {detail}\n"
            f"  {provider.describe()}\n"
            f"  401/403 表示密钥无效：请更新 data/config/secrets.json，或用 --key-env 指向正确的环境变量。"
        ) from None
    except urllib.error.URLError as error:
        raise SystemExit(f"供应商不可达：{provider.base_url}（{error.reason}）") from None


# ---------------------------------------------------------------- 被测服务


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class Server:
    """独立数据目录的被测服务；密钥经环境变量注入，不落盘。"""

    def __init__(self, binary: Path, directory: Path, port: int, provider: Provider):
        self.binary = binary
        self.directory = directory
        self.port = port
        self.provider = provider
        self.process: subprocess.Popen | None = None
        self.base_url = f"http://127.0.0.1:{port}"
        self.data_dir = directory / "data"
        self.data_dir.mkdir(parents=True, exist_ok=True)
        self.log_path = directory / "server.log"
        self.log = self.log_path.open("w", encoding="utf-8")

    def __enter__(self) -> "Server":
        environment = {**os.environ}
        for slot in ("PRIMARY", "ASSIST", "REFLECTION"):
            environment[f"TAVERNAGENT_KEY_{slot}"] = self.provider.key
        self.process = subprocess.Popen(
            [str(self.binary), "-addr", f"127.0.0.1:{self.port}", "-data", str(self.data_dir), "-pin", "482619"],
            cwd=ROOT, env=environment, stdout=self.log, stderr=subprocess.STDOUT,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
        )
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(self.base_url + "/healthz", timeout=2):
                    return self
            except Exception:
                if self.process.poll() is not None:
                    raise SystemExit(f"被测服务启动失败，见 {self.directory / 'server.log'}")
                time.sleep(0.3)
        raise SystemExit("被测服务 45s 内未就绪")

    def __exit__(self, *_):
        if self.process and self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                self.process.kill()
        self.log.close()

    def set_slot(self, slot: str, window: int, max_tokens: int = 4096) -> None:
        # 配置 v2：槽位不再内联连接信息，先建实例再指派槽位。
        configure_slot(Client(self.base_url), slot, name=f"functional-{slot}",
                       kind=self.provider.kind, base_url=self.provider.base_url, model=self.provider.model,
                       temperature=0.6, max_tokens=max_tokens, context_window=window)

    def configure(self, window: int = NORMAL_WINDOW, max_tokens: int = 4096) -> dict:
        # 应用要求窗口与输出预算留出至少 2048 输入 token，收紧窗口时必须同步降输出预算。
        for slot in ("primary", "reflection", "assist"):
            self.set_slot(slot, window, max_tokens)
        return Client(self.base_url).must("GET", "/api/v1/config/provider")


# ---------------------------------------------------------------- 记录器


class Recorder:
    """同时产出人读日志与机读 JSONL。"""

    def __init__(self, directory: Path, header: list[str]):
        self.log_path = directory / "conversation.log"
        self.jsonl_path = directory / "conversation.jsonl"
        self.log = self.log_path.open("w", encoding="utf-8")
        self.jsonl = self.jsonl_path.open("w", encoding="utf-8")
        self.turns: list[dict] = []
        self.line("=" * 72)
        for item in header:
            self.line(item)
        self.line("=" * 72)
        self.line("")

    def close(self) -> None:
        self.log.close()
        self.jsonl.close()

    def line(self, text: str = "") -> None:
        self.log.write(text + "\n")
        self.log.flush()

    def rule(self, title: str) -> None:
        self.line("")
        self.line("─" * 72)
        self.line(title)
        self.line("─" * 72)

    def note(self, text: str) -> None:
        self.line("· " + text)

    def verdict(self, name: str, passed: bool, evidence: str = "") -> None:
        self.line(f"[判定] {'PASS' if passed else 'FAIL'} · {name}" + (f" · {evidence}" if evidence else ""))

    def record_turn(self, record: dict) -> None:
        self.turns.append(record)
        self.jsonl.write(json.dumps(record, ensure_ascii=False) + "\n")
        self.jsonl.flush()
        self.line("")
        self.line(f"[第 {record['index']} 轮 · 玩家]")
        for chunk in record["text"].split("\n"):
            self.line(chunk)
        meta = [f"状态 {record['status']}", f"{record['elapsed']:.1f}s"]
        if record.get("failureCode"):
            meta.append(f"失败原因 {record['failureCode']}: {str(record.get('failureMessage') or '')[:80]}")
        if record.get("repairs"):
            meta.append(f"自动续写 {record['repairs']} 次")
        self.line("")
        self.line(f"[第 {record['index']} 轮 · {record['character']} · " + " · ".join(meta) + "]")
        blocks = record.get("blocks") or []
        if not blocks:
            for chunk in (record.get("prose") or "").split("\n"):
                self.line(chunk)
        for block in blocks:
            text = (block.get("text") or "").strip()
            if not text:
                continue
            kind = block.get("kind", "narration")
            if kind == "dialogue":
                self.line(f"「{text}」")
            elif kind == "inner_monologue":
                self.line(f"（{text}）")
            else:
                self.line(f"*{text}*")


# ---------------------------------------------------------------- 剧本


@dataclass
class Scenario:
    name: str
    title: str
    steps: list[dict] = field(default_factory=list)

    def warn(self, recorder: Recorder, name: str, evidence: str = "") -> None:
        """记录一条观察：留在报告与日志里，但不影响整体判定。"""
        self.steps.append({"name": name, "passed": True, "warning": True, "evidence": evidence})
        recorder.line(f"[提示] {name}" + (f" · {evidence}" if evidence else ""))

    def check(self, recorder: Recorder, name: str, passed: bool, evidence: str = "") -> bool:
        self.steps.append({"name": name, "passed": bool(passed), "evidence": evidence})
        recorder.verdict(name, bool(passed), evidence)
        return bool(passed)

    @property
    def passed(self) -> bool:
        return bool(self.steps) and all(step["passed"] for step in self.steps)


class Runner:
    def __init__(self, server: Server, recorder: Recorder, report: dict):
        self.server = server
        self.client = Client(server.base_url)
        self.recorder = recorder
        self.report = report
        self.counter = 0
        self.card: dict = {}
        self.current: Scenario | None = None
        self.turn_retries = 1

    # ---- 基础操作 ----

    def card_json(self) -> str:
        if not self.card:
            self.card = load_json(BUILTIN_CARD)
        return json.dumps(self.card, ensure_ascii=False)

    def create_session(self, title: str, opening: str | None = None) -> dict:
        if not BUILTIN_CARD.is_file():
            raise SystemExit(f"缺少内置卡 fixture {BUILTIN_CARD}")
        payload = {"title": title, "characterJson": self.card_json(), "playerName": "旅人", "playerRole": "夜里赶路的旅客"}
        if opening:
            payload["openingVariantId"] = opening
        session = self.client.must("POST", "/api/v1/sessions", payload)
        view = self.client.must("GET", "/api/v1/sessions/" + session["sessionId"])
        session["characterId"] = view["characterId"]
        session["state"] = view["state"]
        session["branchId"] = view["branch"]["branchId"]
        return session

    def view(self, session: dict) -> dict:
        return self.client.must("GET", "/api/v1/sessions/" + session["sessionId"])

    def submit(self, session: dict, text: str, label: str) -> dict:
        """走与界面相同的提交路径，并把这一轮写进对话记录。

        真实模型偶发把帧结构写错（例如把 kind 填进 type），应用会直接判回合失败、
        等用户手动重试。测试里记录这次失败并自动重试一次：缺陷留在日志与报告里，
        但不至于让整份功能报告变红。重试后仍失败即按硬失败处理。
        """
        for attempt in range(self.turn_retries + 1):
            result = self._submit_once(session, text, label if attempt == 0 else f"{label}（重试 {attempt}）")
            status = result["record"]["status"]
            if status == "committed" or attempt >= self.turn_retries:
                return result
            reason = record_failure(result)
            if self.current is not None:
                self.current.warn(self.recorder, f"{label}：首答失败（{status}）", reason[:140])
            self.recorder.note(f"{label} 首答失败，自动重试：{reason[:100]}")
        raise AssertionError("unreachable")

    def submit_expect_failure(self, session: dict, text: str, label: str) -> dict:
        """提交一轮并期望它失败（用于锁定"必须显式报错"的契约）。

        不走自动重试：要验证的正是"应用没有替用户吞掉这个失败"。
        """
        return self._submit_once(session, text, label)

    def _submit_once(self, session: dict, text: str, label: str) -> dict:
        """单次提交与等待。"""
        self.counter += 1
        view = self.view(session)
        branch = view["branch"]
        started = time.monotonic()
        accepted = self.client.must("POST", f"/api/v1/sessions/{session['sessionId']}/branches/{branch['branchId']}/turns", {
            "idempotencyKey": uuid.uuid4().hex,
            "expectedHeadId": branch["headNodeId"],
            "expectedVersion": branch["version"],
            "expectedCharacterId": view["characterId"],
            "input": {"kind": "text", "text": text},
        })
        turn = self.client.wait_turn(accepted["turnId"], timeout=180)
        repairs = 0
        while turn.get("status") == "awaiting_continuation" and repairs < 3:
            self.recorder.note(f"正文被截断，自动续写（第 {repairs + 1} 次）")
            self.client.must("POST", f"/api/v1/turns/{accepted['turnId']}/continue",
                             {"expectedCharacterId": view["characterId"]})
            turn = self.client.wait_turn(accepted["turnId"], timeout=180)
            repairs += 1
        elapsed = time.monotonic() - started
        if turn.get("evaluationTimeout"):
            turn = {**turn, "status": "timeout"}
        content = result_content(turn) or {}
        blocks = content.get("blocks") or []
        prose = "\n".join(block.get("text", "") for block in blocks).strip()
        if not prose and turn.get("status") not in (None, "committed"):
            prose = f"[回合未完成] status={turn.get('status')} code={record_failure(turn)}"
        record = {
            "index": self.counter,
            "label": label,
            "failureCode": turn.get("failureCode") or turn.get("failure_code"),
            "failureMessage": turn.get("failureMessage") or turn.get("failure_message"),
            "sessionId": session["sessionId"],
            "turnId": accepted["turnId"],
            "character": view["state"]["characters"].get("npc_chixia", {}).get("name", "角色"),
            "text": text,
            "status": turn.get("status"),
            "elapsed": elapsed,
            "repairs": repairs,
            "blocks": blocks,
            "prose": prose,
            "turnNumber": turn.get("turnNumber"),
        }
        self.recorder.record_turn(record)
        return {**turn, "record": record}

    def raw(self, method: str, path: str, data: bytes | None = None, content_type: str = "application/octet-stream"):
        request = urllib.request.Request(self.server.base_url + path, data=data, method=method,
                                        headers={"Content-Type": content_type})
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.read()

    def summaries(self, session_id: str) -> list:
        with sqlite3.connect(f"file:{self.server.data_dir / 'storage.db'}?mode=ro", uri=True) as db:
            return db.execute(
                "SELECT s.summary_id, s.text FROM summary_artifacts s JOIN plot_nodes p ON p.node_id=s.to_node_id"
                " WHERE p.session_id=?", (session_id,)).fetchall()

    def task_outcomes(self, session_id: str) -> dict:
        """后台任务（reflection/summary）按结果计数，便于解释压缩为什么没落地。"""
        with sqlite3.connect(f"file:{self.server.data_dir / 'storage.db'}?mode=ro", uri=True) as db:
            rows = db.execute(
                "SELECT task, outcome, COUNT(*) FROM turn_usage WHERE task IN ('reflection','summary') GROUP BY task, outcome"
            ).fetchall()
        out: dict = {}
        for task, outcome, count in rows:
            out[f"{task}.{outcome}"] = count
        return out

    def is_ancestor(self, ancestor_id: str, descendant_id: str) -> bool:
        """祖先判定（含自身）。后台记忆提交会合法地把分支头推进到认知节点，
        所以「分叉没动主线」不能直接比较头节点，得看原头是否仍在路径上。"""
        if ancestor_id == descendant_id:
            return True
        with sqlite3.connect(f"file:{self.server.data_dir / 'storage.db'}?mode=ro", uri=True) as db:
            row = db.execute("""
WITH RECURSIVE up(node_key) AS (
  SELECT node_id FROM plot_nodes WHERE node_id = ?
  UNION ALL
  SELECT p.parent_id FROM plot_nodes p JOIN up u ON p.node_id = u.node_key WHERE p.parent_id IS NOT NULL
)
SELECT COUNT(*) FROM up WHERE node_key = ?""", (descendant_id, ancestor_id)).fetchone()
        return bool(row and row[0])

    def settle(self, timeout: float = 25.0) -> None:
        """等后台任务（记忆抽取 / 摘要）落库。"""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            with sqlite3.connect(f"file:{self.server.data_dir / 'storage.db'}?mode=ro", uri=True) as db:
                active = db.execute("SELECT COUNT(*) FROM turn_usage WHERE outcome='in_flight'").fetchone()[0]
            if active == 0:
                return
            time.sleep(0.4)

    def runtime_counters(self) -> dict:
        """读取 /api/status 的运行读数：上下文预算淘汰与摘要维护的观测计数。

        这些计数存在的意义就是让"模型为什么没看到某段历史"可查——把关键项带进
        报告，比让读日志的人自己猜 summary.failed 的含义可靠。
        """
        status = self.client.must("GET", "/api/status")
        counters = status.get("counters") or {}
        keys = ("budgetDroppedHistory", "budgetDroppedSummaries", "budgetUnfit",
                "summaryBlocked", "summaryNormalized", "budgetExceeded")
        return {key: counters.get(key, 0) for key in keys}

    def diagnostics(self) -> dict:
        """读取 /api/status 里的提示词版本与消融开关（ADS-7.8-01 / 7.8-04）。

        证据必须能自证"这一轮跑的是哪份提示词、关了哪些特性"：同一个二进制可以在
        消融模式下启动，报告若不记录这一项，两次结果就无法比较，也无法归因。
        取不到时降级为空字典——观测失败不应该让整轮评测作废。
        """
        try:
            status = self.client.must("GET", "/api/status")
        except (OSError, ValueError, RuntimeError) as error:
            return {"diagnosticError": str(error)}
        out = {key: status[key] for key in ("prompt", "ablation") if key in status}
        phases = (status.get("counters") or {}).get("compilePhases") or {}
        if phases:
            out["compilePhases"] = phases
        return out

    def server_warnings(self, keyword: str = "", limit: int = 0) -> list[str]:
        """被测服务日志里的 WARN/ERROR 行（同文去重计数）。

        后台任务（reflection / summary）的失败在 API 上不可见，只留在服务端日志里；
        把它们带进报告与对话日志，读数才能解释——否则「后台任务 failed 20」这种
        计数看起来像 20 次真实故障，而其中大多数是排队合并时被主动取消的调用。
        """
        return server_log_warnings(self.server.log_path, keyword=keyword, limit=limit)

    # ---- 场景 ----

    def scenario_basic(self, name: str) -> Scenario:
        scenario = Scenario(name, "基础对话")
        self.current = scenario
        session = self.create_session("功能测试 · 基础对话")
        self.recorder.rule(f"场景：{scenario.title}")
        character = session["state"]["characters"]["npc_chixia"]
        self.recorder.note(f"会话 {session['sessionId']}｜角色 {character['name']}（{character.get('nickname') or '无昵称'}）")
        self.recorder.note("开场：" + session["openingText"].strip().replace("\n", " ")[:140])
        scenario.check(self.recorder, "开场正文已生成，且 {{char}}/{{user}} 已展开",
                       bool(session["openingText"].strip()) and "{{" not in session["openingText"],
                       f"{len(session['openingText'])} 字")

        for text, label in (
            ("我踏上跳板，先扶住船舷站稳，再回头看了看那盏亮着的等船灯。", "动作输入"),
            ("「灯是谁点的？」我压低声音问她。", "对白输入"),
            ("我问她，今晚这条船还开不开。", "提问输入"),
        ):
            turn = self.submit(session, text, label)
            record = turn["record"]
            committed = record["status"] == "committed"
            scenario.check(self.recorder, f"{label}：回合提交成功", committed,
                           f"状态 {record['status']}" + (f"（{record.get('failureCode')}: {str(record.get('failureMessage') or '')[:60]}）" if not committed else ""))
            scenario.check(self.recorder, f"{label}：正文非空且无残留宏",
                           committed and len(record["prose"]) >= 30 and "{{" not in record["prose"],
                           f"{len(record['prose'])} 字")
        view = self.view(session)
        committed_turns = sum(1 for record in self.recorder.turns if record.get("sessionId") == session["sessionId"] and record["status"] == "committed")
        nodes = len(view.get("nodes") or [])
        scenario.check(self.recorder, "故事链与成功轮数一致（开场 + 成功轮）",
                       nodes >= 1 + committed_turns, f"节点 {nodes}｜成功轮 {committed_turns}")
        self.report["sessions"]["basic"] = session["sessionId"]
        return scenario

    def scenario_memory(self, name: str) -> Scenario:
        scenario = Scenario(name, "记忆提取与修订")
        self.current = scenario
        session = self.create_session("功能测试 · 记忆")
        sid = session["sessionId"]
        self.recorder.rule(f"场景：{scenario.title}")
        for text in (
            "我把铁皮糖盒放在船板上，说里面的糖可以分她一半。",
            "我告诉她，我叫旅人，从上游的镇子来，明天傍晚要回北码头。",
            "我答应她，收航后陪她把码头上的人重新数一遍。",
            "我问她怕不怕黑。她说不怕，声音却轻了。",
        ):
            self.submit(session, text, "铺垫记忆")
        self.settle()
        page = self.client.must("GET", f"/api/v1/sessions/{sid}/memories?limit=50")
        memories = page.get("memories") or []
        usage = page.get("usage") or {}
        self.recorder.note(f"记忆 {len(memories)} 条｜用量 {json.dumps(usage, ensure_ascii=False)}")
        for memory in memories[:8]:
            self.recorder.note(f"  · [{memory.get('kind')}] {str(memory.get('content'))[:80]}")
        scenario.check(self.recorder, "推进四轮后至少提取到一条记忆", len(memories) >= 1, f"{len(memories)} 条")
        scenario.check(self.recorder, "记忆用量读数可用", "limit" in usage and "used" in usage,
                       json.dumps(usage, ensure_ascii=False)[:80])

        if memories:
            # 修订是 copy-on-write：会生成新 memory_id 并 supersede 旧记录，
            # 因此这里按 subjectKey 追踪"当前生效的那一条"。
            target = next((m for m in memories if m.get("subjectKey")), memories[0])
            subject = target.get("subjectKey") or ""

            def visible(memory_id: str, key: str) -> dict | None:
                """当前生效且未被隐藏的那一条。

                effective 只表示"是该路径上的当前版本"——被隐藏的版本仍然 effective，
                可见性由 hidden 决定（默认列表按 hidden 过滤）。
                """
                page_now = self.client.must("GET", f"/api/v1/sessions/{sid}/memories?limit=100")
                for item in page_now.get("memories") or []:
                    if not item.get("effective") or item.get("hidden"):
                        continue
                    if (key and item.get("subjectKey") == key) or item.get("memoryId") == memory_id:
                        return item
                return None

            def patch_memory(**fields) -> None:
                # 每次修订都会推进分支版本，因此每次都重新取基准。
                latest = self.view(session)
                self.client.must("PATCH", f"/api/v1/sessions/{sid}/branches/{latest['branch']['branchId']}/memories/{target['memoryId']}", {
                    "expectedCharacterId": latest["characterId"],
                    "expectedHeadId": latest["branch"]["headNodeId"],
                    "expectedVersion": latest["branch"]["version"],
                    "idempotencyKey": uuid.uuid4().hex,
                    **fields,
                })

            patch_memory(pinned=True)
            pinned = visible(target["memoryId"], subject)
            scenario.check(self.recorder, "记忆可置顶（修订生成新的覆盖记录）",
                           bool(pinned and pinned.get("pinned")),
                           f"subjectKey={subject or '(无)'}｜pinned={bool(pinned and pinned.get('pinned'))}｜新 memoryId={str((pinned or {}).get('memoryId'))[:20]}")
            patch_memory(pinned=True, hidden=True)
            after_hide = visible(target["memoryId"], subject)
            scenario.check(self.recorder, "隐藏后该记忆不再出现在可见集合", after_hide is None,
                           "已隐藏" if after_hide is None else f"仍可见：{str(after_hide.get('content'))[:40]}")
            hidden_page = self.client.must("GET", f"/api/v1/sessions/{sid}/memories?kind=hidden&limit=100")
            restorable = [m for m in hidden_page.get("memories") or [] if m.get("hidden")]
            scenario.check(self.recorder, "隐藏的记录仍可查询以便恢复", len(restorable) >= 1, f"隐藏区 {len(restorable)} 条")
        self.report["sessions"]["memory"] = sid
        return scenario

    def scenario_compaction(self, name: str) -> Scenario:
        scenario = Scenario(name, "上下文压缩")
        self.current = scenario
        session = self.create_session("功能测试 · 压缩")
        sid = session["sessionId"]
        self.recorder.rule(f"场景：{scenario.title}")
        # 只收紧 primary：摘要由 reflection 槽位生成，它的窗口必须留够，
        # 否则摘要输入超预算 → 摘要任务全失败 → 压缩永远不会落地。
        self.server.set_slot("primary", COMPACTION_WINDOW, COMPACTION_OUTPUT)
        self.server.set_slot("reflection", 131072, 4096)
        self.recorder.note(f"会话 {sid}｜primary contextWindow 收紧到 {COMPACTION_WINDOW}（输出预算 {COMPACTION_OUTPUT}），reflection 保持 131072 用于生成摘要")
        before = len(self.summaries(sid))
        self.recorder.note(f"压缩前摘要工件 {before} 个")
        # 压缩门槛：总轮数需 ≥ 尾部保护窗口 + 8 轮未压缩（MinUncompactedTurns），
        # 因此这里既要推够轮数，也让每轮正文长一些以加快预算耗尽。
        exchanges = [
            "我在船板上坐下，把今天见过的码头编号一个个报给她听：三号、五号、七号，还有最东边那个没有牌子的。",
            "我说起上游那家卖盐味硬糖的铺子，老板姓程，只在清晨开门，糖纸是蓝白条格的。",
            "我提起我的木箱，箱盖上刻着一枚旧船锚，锁扣有些松了，里面装着几封没寄出的信。",
            "我告诉她，我明天傍晚要回北码头，赶最后一班车，车票还压在糖盒底下。",
            "我问她这条航线冬天会不会停，停航的时候她做什么。",
            "我把外套铺在船舷上，说起小时候在这个渡口等过船，那时看船的人还是个老先生。",
            "我说起渡口的旧规矩：只要有人站在码头上，船就不能开，灯也就不能灭。",
            "我请她讲讲自己是怎么开始做摆渡人的，第一班船载的是谁。",
            "我提起雾天行船的难处，问她怎么认方向。",
            "我说起那枚刻痕的木牌，问它是不是每一班都要重新刻。",
            "我把糖盒推到她手边，说剩下的都归她，只留一颗。",
            "我问她，如果今晚真的有第二个乘客，她打算怎么办。",
            "我说我可以留下来陪她数完人数，哪怕天亮。",
            "我提起明天还要赶路，但比起赶车，我更想先弄清这盏灯。",
        ]
        after = before
        for index, text in enumerate(exchanges, start=1):
            turn = self.submit(session, f"{text}（请用四到六段展开这一幕，写清动作、对白与环境。）", f"压缩压力 {index}")
            if turn["record"]["status"] != "committed":
                scenario.check(self.recorder, f"第 {index} 轮提交成功", False, str(turn["record"]["status"]))
                break
            if index % 2 == 0:
                self.settle()
                after = len(self.summaries(sid))
                if after > before:
                    self.recorder.note(f"第 {index} 轮后出现新摘要工件（{before} → {after}）")
                    break
        # 真实模型写摘要时偶发章节嵌套错误（例如 <character_dynamics> 套自身），
        # 应用会丢弃该产物并在后续回合重试；给维护任务几次机会，避免把
        # "这一轮没赶上"误报成"压缩不可用"。
        bonus = 0
        while after == before and bonus < 3:
            bonus += 1
            turn = self.submit(session, "我陪她把最后一盏灯罩擦干净，再核对一遍木牌上的刻痕。（请用三到五段展开。）", f"补推 {bonus}")
            if turn["record"]["status"] != "committed":
                break
            self.settle()
            after = len(self.summaries(sid))
            if after > before:
                self.recorder.note(f"补推 {bonus} 轮后出现新摘要工件（{before} → {after}）")
        tasks = self.task_outcomes(sid)
        summary_warnings = self.server_warnings(keyword="summary", limit=2)
        explanation = "｜服务端告警 " + "；".join(summary_warnings) if summary_warnings else ""
        scenario.check(self.recorder, "窗口收紧后生成摘要工件（压缩生效）", after > before,
                       f"摘要工件 {before} → {after}（含补推 {bonus} 轮）｜后台任务 {json.dumps(tasks, ensure_ascii=False)}{explanation}")

        if after > before:
            rows = self.summaries(sid)
            self.recorder.note("最近摘要摘录：" + str(rows[-1][1]).replace("\n", " ")[:200])
        else:
            self.recorder.note("未落地的后台任务多数是排队合并时被主动取消的调用（同一分支只保留最新一次），"
                               "用量账本把这类取消记为 failed，计数偏高不等于故障次数")
        view = self.view(session)
        marker = view.get("activeSummary") or view.get("summary")
        if marker:
            scenario.check(self.recorder, "会话视图带生效中的摘要标记", True,
                           json.dumps(marker, ensure_ascii=False)[:80])
        else:
            # 摘要工件已落库却不出现在视图里：按技术契约 §9.2，区间内的记忆被后续修订改写
            # （内容不同或被隐藏）时旧摘要会被主动丢弃——这是文档化设计，记为提示而非失败。
            scenario.warn(self.recorder, "会话视图未返回生效中的摘要",
                          f"摘要工件 {after} 个｜可能被区间内的记忆修订失效，或尚未落地")
        turn = self.submit(session, "请回忆：我打算明天傍晚去哪里，我的木箱盖上刻着什么？", "压缩后回忆关键事实")
        prose = turn["record"]["prose"]
        hit_place, hit_box = "北码头" in prose, "锚" in prose
        scenario.check(self.recorder, "压缩后仍答得出早期关键事实（北码头 / 船锚）", hit_place and hit_box,
                       f"地点{'命中' if hit_place else '未命中'}｜木箱{'命中' if hit_box else '未命中'}｜"
                       + prose[:80].replace("\n", " "))

        # 承重摘要装不下时必须显式报错：把窗口压到装不下摘要，验证应用不再静默丢弃
        # （丢摘要 = 模型彻底失去那一段历史，正是"角色突然失忆"的根因）。
        if after > before and marker:
            self.server.set_slot("primary", CONSTRAINT_WINDOW, COMPACTION_OUTPUT)
            self.recorder.note(f"把 primary 窗口压到 {CONSTRAINT_WINDOW}，让承重摘要装不下")
            failure = self.submit_expect_failure(session, "我继续把灯罩擦干净。", "窗口不足时的显式报错")
            code = failure["record"]["failureCode"] or ""
            message = str(failure["record"]["failureMessage"] or "")
            scenario.check(self.recorder, "承重摘要装不下时显式报错（不静默丢弃历史）",
                           code == "CONTEXT_OVER_BUDGET" and ("摘要" in message or "预算" in message),
                           f"{code}: {message[:140]}")
            # 恢复窗口后必须能继续：这是配置问题，不是故事损坏。
            self.server.configure(NORMAL_WINDOW, 4096)
            resumed = self.submit(session, "我把灯罩放回去，抬眼看她。", "恢复窗口后继续")
            scenario.check(self.recorder, "恢复窗口后故事继续（错误可恢复）",
                           resumed["record"]["status"] == "committed", str(resumed["record"]["status"]))
        else:
            scenario.warn(self.recorder, "未验证「装不下 → 显式报错」",
                          "本次没有生效摘要（工件或视图标记缺失），跳过该步骤")
        counters = self.runtime_counters()
        self.recorder.note("运行读数：" + json.dumps(counters, ensure_ascii=False))
        scenario.check(self.recorder, "运行读数可读（预算淘汰/摘要维护计数）", bool(counters),
                       json.dumps(counters, ensure_ascii=False))
        self.report["counters"] = counters
        self.server.configure(NORMAL_WINDOW, 4096)
        self.report["sessions"]["compaction"] = sid
        return scenario

    def scenario_director(self, name: str) -> Scenario:
        scenario = Scenario(name, "导演模式")
        self.current = scenario
        session = self.create_session("功能测试 · 导演模式")
        sid, branch = session["sessionId"], session["branchId"]
        self.recorder.rule(f"场景：{scenario.title}")
        self.recorder.note(f"会话 {sid}")

        def director() -> dict:
            return self.client.must("GET", f"/api/v1/sessions/{sid}/branches/{branch}/director")

        initial = director()
        scenario.check(self.recorder, "导演状态可读且初始未启用",
                       not (initial.get("state") or {}).get("status"), json.dumps(initial.get("state"), ensure_ascii=False)[:60])

        view = self.view(session)
        request = self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{branch}/director/messages", {
            "expectedCharacterId": view["characterId"],
            "expectedHeadId": view["branch"]["headNodeId"],
            "expectedDraftVersion": (initial.get("draft") or {}).get("version", 0),
            "idempotencyKey": uuid.uuid4().hex,
            "text": "请把今晚这件事安排成两个阶段：先查清等船灯为什么亮，再决定要不要开船。",
        })
        deadline = time.monotonic() + 180
        result = {}
        while time.monotonic() < deadline:
            result = self.client.must("GET", f"/api/v1/director-requests/{request['requestId']}")
            if result.get("status") in ("completed", "failed", "cancelled", "interrupted"):
                break
            time.sleep(0.4)
        self.recorder.note(f"导演讨论：status={result.get('status')} draftApplied={result.get('draftApplied')}")
        if result.get("reply"):
            self.recorder.note("导演回复：" + str(result["reply"]).replace("\n", " ")[:220])
        applied = bool(result.get("draftApplied"))
        candidate = result.get("candidate") or {}
        beats = len((candidate.get("beats") or [])) if isinstance(candidate, dict) else 0
        reply_text = str(result.get("reply") or "").strip()
        # 讨论的硬契约是「完成 + 有回复」；是否附带大纲候选取决于模型当轮的取舍
        # （讨论本来就是来回对话，模型可以先反问再给方案），所以候选缺失只是提示。
        scenario.check(self.recorder, "导演讨论得到模型回复",
                       result.get("status") == "completed" and bool(reply_text),
                       f"status={result.get('status')} 回复 {len(reply_text)} 字 draftApplied={applied} 候选阶段数={beats}")
        if not (applied or beats):
            scenario.warn(self.recorder, "本轮讨论未附带大纲候选（模型只给了文字回复）",
                          "链路可用：草稿可手动编辑，或再讨论一轮让模型给出分阶段方案")

        current = director()
        draft = current.get("draft") or {}
        plan = dict(draft.get("plan") or {})
        beats = list(plan.get("beats") or [])
        plan["title"] = plan.get("title") or "等船灯之夜"
        plan["guidance"] = plan.get("guidance") or "保持池夏的口吻：嘴上逞强，手上的活计却在照顾旅人。"
        plan["beats"] = (beats + [
            {"beatId": "beat-probe", "title": "查清等船灯",
             "instruction": "让池夏带旅人核对人数、灯号记录与木牌。", "completionCriteria": "查明等船灯为什么会亮。"},
            {"beatId": "beat-decide", "title": "决定是否开船",
             "instruction": "只剩最后一个乘客时，让池夏自己做出选择。", "completionCriteria": "给出明确决定并收束这一幕。"},
        ])[:2]
        for index, beat in enumerate(plan["beats"]):
            beat.setdefault("beatId", f"beat-{index}")
            beat.setdefault("title", f"阶段 {index + 1}")
            beat.setdefault("instruction", "按前一阶段的结果推进。")
            beat.setdefault("completionCriteria", "阶段目标达成。")
        self.recorder.note("大纲：" + str(plan["title"]) + "｜阶段：" + " / ".join(str(b["title"]) for b in plan["beats"]))
        saved = self.client.must("PUT", f"/api/v1/sessions/{sid}/branches/{branch}/director/draft", {
            "expectedCharacterId": view["characterId"],
            "expectedDraftVersion": draft.get("version", 0),
            "baseRevisionId": draft.get("baseRevisionId", ""),
            "plan": plan,
        })
        scenario.check(self.recorder, "大纲可编辑保存（草稿版本前进）",
                       int(saved.get("version", 0)) > int(draft.get("version", 0) or 0),
                       f"版本 {int(draft.get('version', 0) or 0)} → {saved.get('version')}")

        view = self.view(session)
        activated = self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{branch}/director/commands", {
            "action": "activate", "draftVersion": saved.get("version"), "expectedCharacterId": view["characterId"],
            "expectedHeadId": view["branch"]["headNodeId"], "expectedVersion": view["branch"]["version"],
            "idempotencyKey": uuid.uuid4().hex,
        })
        state = activated.get("state") or {}
        scenario.check(self.recorder, "大纲激活后进入 active 且当前阶段明确",
                       state.get("status") == "active" and bool(state.get("currentBeatId")),
                       f"status={state.get('status')} currentBeatId={state.get('currentBeatId')}")

        turn = self.submit(session, "我跟着她把码头尽头的灯罩摘下来，一起检查灯芯和木牌上的记录。", "大纲进行中的一轮")
        scenario.check(self.recorder, "启用大纲后仍能正常推进回合", turn["record"]["status"] == "committed", str(turn["record"]["status"]))

        for action, expect in (("pause", "paused"), ("resume", "active")):
            view = self.view(session)
            current = director()
            commanded = self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{branch}/director/commands", {
                "action": action, "draftVersion": (current.get("draft") or {}).get("version", 0),
                "expectedCharacterId": view["characterId"], "expectedHeadId": view["branch"]["headNodeId"],
                "expectedVersion": view["branch"]["version"], "idempotencyKey": uuid.uuid4().hex,
            })
            status = (commanded.get("state") or {}).get("status")
            scenario.check(self.recorder, f"导演命令 {action} 生效", status == expect, f"status={status}")

        current = director()
        beat_id = ((current.get("state") or {}).get("currentBeatId")) or None
        view = self.view(session)
        completed = self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{branch}/director/commands", {
            "action": "complete", "beatId": beat_id, "draftVersion": (current.get("draft") or {}).get("version", 0),
            "expectedCharacterId": view["characterId"], "expectedHeadId": view["branch"]["headNodeId"],
            "expectedVersion": view["branch"]["version"], "idempotencyKey": uuid.uuid4().hex,
        })
        progress = (completed.get("state") or {}).get("progress") or {}
        done = [beat for beat, value in progress.items() if value.get("status") == "completed"]
        scenario.check(self.recorder, "可手动完成当前阶段并记录进度", bool(done),
                       f"已完成 {done}｜warning={(completed.get('state') or {}).get('warning') or '无'}")
        self.report["sessions"]["director"] = sid
        return scenario

    def scenario_lorebook(self, name: str) -> Scenario:
        scenario = Scenario(name, "世界书")
        self.current = scenario
        session = self.create_session("功能测试 · 世界书")
        self.recorder.rule(f"场景：{scenario.title}")
        page = self.client.must("GET", f"/api/v1/sessions/{session['sessionId']}/lorebook?limit=50")
        entries = [item.get("entry", {}) for item in page.get("entries") or []]
        self.recorder.note("世界书：" + " / ".join(f"{e.get('title')}（{'、'.join(e.get('keys') or [])}）" for e in entries))
        scenario.check(self.recorder, "内置卡的世界书已随卡导入", len(entries) >= 3, f"{len(entries)} 条")
        scenario.check(self.recorder, "条目带关键词与正文",
                       all(e.get("keys") and e.get("content") for e in entries),
                       f"首条关键词 {'、'.join(entries[0]['keys'])}" if entries else "无条目")
        turn = self.submit(session, "我指着码头尽头的等船灯，问它按规矩什么时候该点亮。", "命中世界书关键词的一轮")
        scenario.check(self.recorder, "命中关键词的回合正常完成", turn["record"]["status"] == "committed",
                       str(turn["record"]["status"]))
        self.recorder.note("提示词级注入取证需要计量代理模式（--meter）")
        self.report["sessions"]["lorebook"] = session["sessionId"]
        return scenario

    def scenario_branch(self, name: str) -> Scenario:
        scenario = Scenario(name, "分支分叉")
        self.current = scenario
        session = self.create_session("功能测试 · 分支")
        sid, branch = session["sessionId"], session["branchId"]
        self.recorder.rule(f"场景：{scenario.title}")
        first = self.submit(session, "我留在船上，先检查缆绳。", "主线第一轮")
        view = self.view(session)
        main_head = view["branch"]["headNodeId"]
        forked = self.client.must("POST", f"/api/v1/sessions/{sid}/branches", {
            "fromNodeId": session["rootNodeId"], "name": "另一条路", "expectedCharacterId": view["characterId"],
        })
        fork_id = forked.get("branchId") or (forked.get("branch") or {}).get("branchId")
        scenario.check(self.recorder, "可以从开场节点分叉出新分支", bool(fork_id), f"branchId={fork_id}")
        if not fork_id:
            self.report["sessions"]["branch"] = sid
            return scenario
        fork_view = self.client.must("GET", f"/api/v1/sessions/{sid}?branchId={fork_id}")
        fork_nodes = len(fork_view.get("nodes") or [])
        scenario.check(self.recorder, "新分支从开场重新开始（不含主线那一轮）", fork_nodes == 1, f"节点 {fork_nodes}")
        forked_turn = self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{fork_id}/turns", {
            "idempotencyKey": uuid.uuid4().hex,
            "expectedHeadId": fork_view["branch"]["headNodeId"],
            "expectedVersion": fork_view["branch"]["version"],
            "expectedCharacterId": fork_view["characterId"],
            "input": {"kind": "text", "text": "我走上码头，径直去查看那盏灯。"},
        })
        fork_after = self.client.wait_turn(forked_turn["turnId"], timeout=180)
        scenario.check(self.recorder, "分叉分支可独立推进", fork_after.get("status") == "committed", str(fork_after.get("status")))
        # 显式按主线分支读取：分叉后默认视图可能已经切到新分支。
        main_after = self.client.must("GET", f"/api/v1/sessions/{sid}?branchId={branch}")
        main_head_after = main_after["branch"]["headNodeId"]
        untouched = self.is_ancestor(main_head, main_head_after)
        scenario.check(self.recorder, "分叉不影响主线头节点与内容",
                       untouched and len(main_after.get("nodes") or []) == 2,
                       f"主线头{'未回退' if untouched else '被改写'}（{main_head[:12]} → {main_head_after[:12]}）｜主线节点 {len(main_after.get('nodes') or [])}")
        scenario.check(self.recorder, "两条分支头节点不同",
                       main_head_after != fork_after.get("resultNodeId"),
                       f"主线 {main_head_after[:12]}｜分叉 {str(fork_after.get('resultNodeId'))[:12]}")
        self.report["sessions"]["branch"] = sid
        return scenario

    def scenario_pack(self, name: str) -> Scenario:
        scenario = Scenario(name, "剧情包往返")
        self.current = scenario
        session = self.create_session("功能测试 · 剧情包")
        sid = session["sessionId"]
        self.recorder.rule(f"场景：{scenario.title}")
        self.submit(session, "我把糖盒放在船板上，说换她讲讲这条航线。", "导出前一轮")
        before = self.view(session)
        status, blob = self.raw("GET", f"/api/v1/sessions/{sid}/export")
        scenario.check(self.recorder, "可以导出剧情包", status == 200 and len(blob) > 0, f"{len(blob)} 字节")
        status, imported = self.raw("POST", "/api/v1/sessions/import", blob, "application/zip")
        payload = {}
        if 200 <= status < 300:
            try:
                payload = json.loads(imported.decode("utf-8"))
            except ValueError:
                payload = {}
        scenario.check(self.recorder, "剧情包可以重新导入", bool(payload.get("sessionId")), f"HTTP {status}")
        if payload.get("sessionId"):
            after = self.client.must("GET", f"/api/v1/sessions/{payload['sessionId']}")
            same_title = after.get("title") == before.get("title")
            same_nodes = len(after.get("nodes") or []) == len(before.get("nodes") or [])
            scenario.check(self.recorder, "导入后标题与回合数一致", same_title and same_nodes,
                           f"标题{'一致' if same_title else '不一致'}｜节点 {len(after.get('nodes') or [])}/{len(before.get('nodes') or [])}")
        self.report["sessions"]["pack"] = sid
        return scenario


# ---------------------------------------------------------------- 入口


def record_failure(turn: dict) -> str:
    code = turn.get("failureCode") or turn.get("failure_code") or "UNKNOWN"
    message = turn.get("failureMessage") or turn.get("failure_message") or ""
    return f"{code}: {message}".strip(": ")


def attribute_failed_steps(report: dict) -> list[dict]:
    """把失败项整理成结构化归因（ADS-7.4-09）。

    只填**已知**的字段：功能场景的判定是断言式的，脚本没有"首个错误步"的定位能力，
    因此 firstErrorStep 固定为 assert、rootCauseSide 保持 unknown 并标注 confidence=low。
    宁可如实说"未定位"，也不要按关键词猜一个看起来专业的类别——那会让读报告的人
    以为已经做过归因，从而跳过回放轨迹这一步。
    """
    attributions = []
    for scenario in report.get("scenarios", []):
        if scenario.get("passed"):
            continue
        for step in scenario.get("steps", []):
            if step.get("passed"):
                continue
            attributions.append({
                "scenario": scenario.get("name"),
                "taskGoal": scenario.get("title") or scenario.get("name"),
                "step": step.get("name"),
                "firstErrorStep": "assert",
                "errorClass": "scenario_assertion_failed",
                "rootCauseSide": "unknown",
                "confidence": "low",
                "recoverable": True,
                "rootCauseNote": "场景断言失败；脚本未做首错定位，需回放 conversation.log 与轨迹判定「看/想/做/验」哪一层出问题（ADS-7.9.1）",
                "evidence": {"detail": str(step.get("evidence", ""))[:400]},
            })
    return attributions


def server_log_warnings(path: Path, keyword: str = "", limit: int = 0) -> list[str]:
    """按出现次数汇总被测服务日志里的 WARN/ERROR 行（同文合并，次数降序）。"""
    if not Path(path).is_file():
        return []
    counts: dict[str, int] = {}
    for line in Path(path).read_text(encoding="utf-8", errors="replace").splitlines():
        if " WARN " not in line and " ERROR " not in line:
            continue
        if keyword and keyword not in line:
            continue
        body = line.split(" ", 2)[-1].strip()
        counts[body] = counts.get(body, 0) + 1
    items = sorted(counts.items(), key=lambda item: (-item[1], item[0]))
    if limit > 0:
        items = items[:limit]
    return [f"{count}× {body}" for body, count in items]


def describe_slots(config: dict) -> str:
    """把 PUT /config/provider 的回执压成一行；顺带暴露"密钥没被应用进程拿到"这种问题。"""
    providers = config.get("providers")
    items = list(providers.values()) if isinstance(providers, dict) else list(providers or [])
    labels = []
    for item in items:
        if not isinstance(item, dict):
            continue
        labels.append(f"{item.get('slot')}（{'启用' if item.get('enabled') else '停用'}，{item.get('model') or item.get('kind')}，密钥{'已注入' if item.get('hasApiKey') else '缺失'}）")
    return "、".join(labels) or "无"


def git_commit() -> str:
    try:
        return subprocess.check_output(["git", "rev-parse", "--short=12", "HEAD"], cwd=ROOT, text=True).strip()
    except Exception:
        return "unknown"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--provider", choices=("configured", "gemini"), default="configured",
                        help="configured=用 data/config 里已配置的供应商；gemini=本地 Gemini 网关（需 --key-env）")
    parser.add_argument("--base-url", default="", help="覆盖供应商地址（OpenAI 兼容）")
    parser.add_argument("--model", default="", help="覆盖模型名")
    parser.add_argument("--key-env", default="", help="从哪个环境变量读取密钥")
    parser.add_argument("--kind", default="", help="协议类型：openai-chat / openai-responses / anthropic-messages")
    parser.add_argument("--binary", type=Path, default=DEFAULT_BINARY)
    parser.add_argument("--directory", type=Path, default=None, help="产物目录（默认 output/functional/<时间戳>-<provider>）")
    parser.add_argument("--port", type=int, default=0, help="被测服务端口（默认随机空闲端口）")
    parser.add_argument("--scenarios", default=",".join(SCENARIOS), help="要运行的场景，逗号分隔：" + ", ".join(SCENARIOS))
    parser.add_argument("--skip-preflight", action="store_true", help="跳过 /models 预检（仅在供应商不提供该端点时使用）")
    parser.add_argument("--turn-retries", type=int, default=2, help="单轮失败后的自动重试次数（默认 2，失败仍留痕）")
    args = parser.parse_args()

    binary = args.binary.resolve()
    # 先校验纯输入（场景名），再检查外部依赖（二进制）：非法参数应当在
    # 动文件系统之前就报错，也让测试无需先构建产物。
    selected = [name.strip() for name in args.scenarios.split(",") if name.strip()]
    unknown = [name for name in selected if name not in SCENARIOS]
    if unknown:
        raise SystemExit("未知场景：" + ", ".join(unknown))
    if not binary.is_file():
        raise SystemExit(f"找不到被测二进制 {binary}；先执行 powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Version dev")

    provider = resolve_provider(args)
    print(f"供应商：{provider.describe()}", flush=True)
    if not args.skip_preflight:
        preflight(provider)
        print("预检通过", flush=True)

    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    directory = (args.directory or (ROOT / "output/functional" / f"{stamp}-{provider.label}")).resolve()
    directory.mkdir(parents=True, exist_ok=True)
    fingerprint = {"binary": str(binary.relative_to(ROOT)) if binary.is_relative_to(ROOT) else str(binary),
                   "binarySHA256": digest(binary), "commit": git_commit()}
    card = load_json(BUILTIN_CARD)
    report = {
        "generatedAt": utc(),
        "provider": {"kind": provider.kind, "baseUrl": provider.base_url, "model": provider.model,
                     "label": provider.label, "source": provider.source},
        "fingerprint": fingerprint,
        "characterCard": {"fixture": str(BUILTIN_CARD.relative_to(ROOT)), "cardId": card.get("cardId"), "name": card.get("name")},
        "compactionWindow": {"contextWindow": COMPACTION_WINDOW, "maxTokens": COMPACTION_OUTPUT},
        "scenarios": [], "sessions": {}, "passed": False, "completed": False, "serverWarnings": [],
    }
    runner = None
    recorder = Recorder(directory, [
        "功能测试对话记录",
        f"模型：{provider.model} @ {provider.base_url}（kind={provider.kind}）",
        f"供应商来源：{provider.source}",
        f"被测二进制：{fingerprint['binary']}（sha256 {fingerprint['binarySHA256'][:12]}，commit {fingerprint['commit']}）",
        f"角色卡：{card.get('name')}（{card.get('cardId')}）",
        f"开始时间：{report['generatedAt']}",
        f"场景：{'、'.join(selected)}",
    ])

    try:
        with Server(binary, directory, args.port or free_port(), provider) as server:
            recorder.note(f"被测服务：{server.base_url}（数据目录 {server.data_dir.relative_to(ROOT)}）")
            config = server.configure()
            recorder.note("已配置槽位：" + describe_slots(config))
            runner = Runner(server, recorder, report)
            runner.turn_retries = max(0, args.turn_retries)
            for name in selected:
                try:
                    item = getattr(runner, f"scenario_{name}")(name)
                except Exception as error:  # 一个场景炸掉不该让整轮作废：记账后继续
                    recorder.rule(f"场景异常：{name}")
                    recorder.note(f"{type(error).__name__}: {error}")
                    item = Scenario(name, f"{name}（执行异常）")
                    item.steps.append({"name": "场景执行异常", "passed": False,
                                       "evidence": f"{type(error).__name__}: {error}"[:300]})
                report["scenarios"].append({"name": item.name, "title": item.title, "passed": item.passed, "steps": item.steps})
                recorder.rule(f"场景小结：{item.title} · " + ("全部通过" if item.passed else "存在失败项") +
                              f"（{sum(1 for s in item.steps if s['passed'])}/{len(item.steps)}）")
            report["completed"] = True
    finally:
        report["passed"] = report["completed"] and bool(report["scenarios"]) and all(s["passed"] for s in report["scenarios"])
        report["turnCount"] = len(recorder.turns)
        report["finishedAt"] = utc()
        report["serverWarnings"] = server_log_warnings(directory / "server.log")
        report["failureAttributions"] = attribute_failed_steps(report)
        if runner is not None:
            report["diagnostics"] = runner.diagnostics()
        total = sum(len(s["steps"]) for s in report["scenarios"])
        failed = sum(1 for s in report["scenarios"] for step in s["steps"] if not step["passed"])
        recorder.rule("总结")
        recorder.line(f"场景 {len(report['scenarios'])} 个｜判定 {total} 项｜通过 {total - failed}｜失败 {failed}｜对话 {report['turnCount']} 轮")
        diagnostics = report.get("diagnostics") or {}
        if diagnostics:
            recorder.line(f"提示词版本 {diagnostics.get('prompt', '未知')}｜消融开关 {diagnostics.get('ablation', '未知')}")
        for item in report["scenarios"]:
            recorder.line(f"  · {item['title']}：{'通过' if item['passed'] else '存在失败项'}")
        if report["serverWarnings"]:
            recorder.line("")
            recorder.line("服务端告警（WARN/ERROR，按出现次数）：")
            for line in report["serverWarnings"]:
                recorder.line(f"  · {line}")
        recorder.line("")
        recorder.line(f"完整对话记录：{recorder.log_path}")
        recorder.line(f"结构化记录：{recorder.jsonl_path}")
        recorder.line(f"判定报告：{directory / 'report.json'}")
        recorder.close()
        (directory / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"passed": report["passed"], "scenarios": len(report["scenarios"]), "turns": report["turnCount"],
                          "directory": str(directory), "log": str(recorder.log_path)}, ensure_ascii=False), flush=True)
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
