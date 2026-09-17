"""Real-provider feature chains, using only the isolated metered evaluator."""
import base64
import concurrent.futures
import hashlib
import json
import sqlite3
import struct
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zlib

from eval_protocol import result_content
from gemini_meter import utc


class Features:
    def __init__(self, evaluation, name):
        self.e = evaluation
        self.client = evaluation.client
        self.path = evaluation.directory / (name + ("-" + evaluation.args.label if evaluation.args.label else "") + ".json")
        if self.path.exists():
            raise RuntimeError("Preserve the first report; choose another evaluation directory for a retest")
        from release_eval import fingerprint
        from gemini_meter import MODEL
        self.report = {"generatedAt": utc(), "phase": name, "model": MODEL,
                       "fingerprint": fingerprint(evaluation.binary), "cases": [], "turns": []}
        meter_phase = "preflight" if name == "compatibility" else "long_story" if name.startswith("long-story") else name.replace("-", "_")
        self.e.meter("/control/phase", {"phase": meter_phase})

    def save(self):
        from release_eval import write_json
        self.report["ledger"] = self.e.save_ledger()
        self.report["passed"] = self.report.get("completed", False) and bool(self.report["cases"]) and all(x["passed"] for x in self.report["cases"])
        write_json(self.path, self.report)

    def check(self, name, condition, evidence=None):
        self.report["cases"].append({"name": name, "passed": bool(condition), "evidence": evidence})
        self.save()
        print(json.dumps({"case": name, "passed": bool(condition), **self.report["ledger"]}, ensure_ascii=False), flush=True)
        return condition

    def raw(self, method, path, data=None, content_type="application/octet-stream"):
        req = urllib.request.Request(self.client.base_url + path, data=data, method=method,
                                     headers={"Content-Type": content_type})
        try:
            with urllib.request.urlopen(req, timeout=30) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.read()

    def create(self, title):
        from release_eval import ROOT
        card = json.loads((ROOT / "scripts/fixtures/quality_story_card.json").read_text(encoding="utf-8-sig"))
        card["cardId"] = "release_" + uuid.uuid4().hex
        card["description"] = "原创海港故事。扮演灯塔向导，与旅人苏行探索海港。每轮使用两至四段简洁中文叙述和对白，遵守系统结构化协议。"
        payload = {"title": title, "characterJson": json.dumps(card, ensure_ascii=False),
                   "playerName": "苏行", "playerRole": "海港档案核对员", "openingVariantId": "harbor"}
        created = self.client.must("POST", "/api/v1/sessions", payload)
        view = self.client.must("GET", "/api/v1/sessions/" + created["sessionId"])
        view["openingText"] = created["openingText"]
        return view, card

    def view(self, session, branch=None):
        bid = branch or session["branch"]["branchId"]
        return self.client.must("GET", f"/api/v1/sessions/{session['sessionId']}?branchId={bid}")

    def settle(self, session, accepted, name):
        turn = self.client.wait_turn(accepted["turnId"], 180)
        first = turn.get("status")
        repairs = 0
        if first == "awaiting_continuation":
            repairs = 1
            self.client.must("POST", f"/api/v1/turns/{turn['turnId']}/continue", {"expectedCharacterId": session["characterId"]})
            turn = self.client.wait_turn(turn["turnId"], 180)
        self.report["turns"].append({"name": name, "initialStatus": first, "repairs": repairs, "result": turn})
        self.check(name, turn.get("status") == "committed" and bool(result_content(turn).get("blocks")),
                   {"turnId": turn["turnId"], "initialStatus": first, "status": turn.get("status"), "repairs": repairs})
        if turn.get("status") != "committed":
            self.client.cancel_unfinished(turn["turnId"], session["characterId"])
            raise RuntimeError(name + " did not commit")
        return turn

    def turn(self, session, text, name, kind="text", **extra):
        view = self.view(session)
        branch = view["branch"]
        payload = {"idempotencyKey": uuid.uuid4().hex, "expectedHeadId": branch["headNodeId"],
                   "expectedVersion": branch["version"], "expectedCharacterId": view["characterId"],
                   "mode": "structured", "input": {"kind": kind, "text": text, **extra}}
        path = f"/api/v1/sessions/{view['sessionId']}/branches/{branch['branchId']}/turns"
        accepted = self.client.must("POST", path, payload)
        repeated = self.client.must("POST", path, payload)
        self.check(name + "/idempotency", repeated["turnId"] == accepted["turnId"])
        return self.settle(view, accepted, name)

    def replay(self, turn):
        def collect(after=""):
            request = urllib.request.Request(self.client.base_url+f"/api/v1/turns/{turn['turnId']}/events",
                                             headers={"Last-Event-ID": after} if after else {})
            events, event = [], {}
            with urllib.request.urlopen(request, timeout=15) as response:
                for line in response:
                    line = line.decode("utf-8").strip()
                    if line.startswith("id:"):
                        event["id"] = line[3:].strip()
                    elif line.startswith("event:"):
                        event["type"] = line[6:].strip()
                    elif line.startswith("data:"):
                        event["data"] = line[5:].strip()
                    elif not line and event:
                        events.append(event)
                        if event.get("type") in ("turn.committed", "turn.failed", "turn.cancelled"):
                            break
                        event = {}
            return events
        original = collect()
        self.check("SSE durable replay", bool(original) and original[-1].get("type") == "turn.committed", original)
        if len(original) > 1:
            resumed = collect(original[0]["id"])
            self.check("SSE reconnect cursor", resumed == original[1:], resumed)

    def run(self):
        self.e.configure(reflection=False)
        session, card = self.create("Gemini feature acceptance")
        self.report["sessionId"] = session["sessionId"]
        sid, cid, bid = session["sessionId"], session["characterId"], session["branch"]["branchId"]
        base = f"/api/v1/sessions/{sid}/branches/{bid}"
        self.check("opening and player identity", session["state"]["items"]["quality_tea"]["quantity"] == 5 and "晨光" in session["openingText"])
        for fmt in ("json", "png"):
            blob = json.dumps(card, ensure_ascii=False).encode()
            if fmt == "png":
                def chunk(kind, value):
                    return struct.pack(">I", len(value)) + kind + value + struct.pack(">I", zlib.crc32(kind + value))
                blob = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)) + chunk(b"tEXt", b"chara\0"+base64.b64encode(blob)) + chunk(b"IDAT", zlib.compress(b"\0\xff\xff\xff")) + chunk(b"IEND", b"")
            status, raw = self.raw("POST", "/api/v1/cards/import", blob)
            self.check(fmt + " character import", status == 200 and json.loads(raw).get("name") == card["name"])
        bad, _ = self.raw("POST", "/api/v1/cards/import", b"not a card")
        self.check("invalid character rejected", bad == 422)
        second, _ = self.create("Same name, separate identity")
        self.check("same-name identity isolation", second["characterId"] != cid)
        first = self.turn(session, "向导，我叫苏行。我把备用钥匙放在蓝色盒子里，约好明晚在北码头会合。请记住这些安排，告诉我你的看法。", "free input")
        self.replay(first)
        late = self.client.must("POST", f"/api/v1/turns/{first['turnId']}/cancel", {"expectedCharacterId": cid})
        self.check("late cancellation preserves commit", late.get("status") == "committed" and self.client.must("GET", f"/api/v1/turns/{first['turnId']}")["resultNodeId"] == first["resultNodeId"])
        options = result_content(first).get("options", [])
        self.check("generated options", bool(options))
        if options:
            option = options[0]
            self.turn(session, option["text"], "option selection", kind="option", optionRef={"nodeId": first["resultNodeId"], "optionId": option["optionId"]})
        action = self.turn(session, "我饮用两份旅行茶，然后保管好剩余物品。", "rule action", kind="action", actionRef="drink.tea")
        self.check("rule inventory settles once", self.view(session)["state"]["items"]["quality_tea"]["quantity"] == 3)
        snapshot = self.client.must("GET", f"/api/v1/nodes/{action['resultNodeId']}")
        for recheck in (False, True):
            accepted = self.client.must("POST", base+"/regenerations", {"nodeId": action["resultNodeId"], "recheck": recheck, "idempotencyKey": uuid.uuid4().hex, "expectedCharacterId": cid})
            result = self.settle(session, accepted, "recheck" if recheck else "regenerate")
            self.check("derived action consumption", self.view(session, accepted["branchId"])["state"]["items"]["quality_tea"]["quantity"] == 3)
            if not recheck:
                self.report["regeneratedNode"] = result["resultNodeId"]
        edited = self.client.must("POST", base+"/edits", {"nodeId": action["resultNodeId"], "blocks": [{"kind": "narration", "text": "苏行把茶重新放好，决定暂时休息。"}], "expectedCharacterId": cid})
        self.check("assistant edit creates narrative branch", edited.get("mode") == "narrative" and self.view(session, edited["branchId"])["state"]["items"]["quality_tea"]["quantity"] == 5)
        now = self.client.must("GET", f"/api/v1/nodes/{action['resultNodeId']}")
        self.check("original history immutable", all(snapshot[key] == now[key] for key in ("node", "state")))
        accepted = self.client.must("POST", base+"/edits", {"nodeId": first["resultNodeId"], "input": {"kind": "text", "text": "我想先听听海港的风声。"}, "idempotencyKey": uuid.uuid4().hex, "expectedCharacterId": cid})
        self.settle(session, accepted, "player input edit")
        readonly = self.client.must("GET", f"/api/v1/sessions/{sid}?branchId={bid}&viewNodeId={session['rootNodeId']}")
        self.check("history view retains original inventory", readonly["state"]["items"]["quality_tea"]["quantity"] == 5)
        foreign, _ = self.client.request("POST", base+"/turns", {"idempotencyKey": uuid.uuid4().hex, "expectedCharacterId": second["characterId"], "expectedHeadId": self.view(session)["branch"]["headNodeId"], "input": {"text": "越界输入"}})
        self.check("cross-character write rejected", foreign in (400, 409, 422))
        self.memory(session)
        self.director(session)
        self.check("graph and lorebook", bool(self.client.must("GET", f"/api/v1/sessions/{sid}/graph?branchId={bid}").get("nodes")) and "entries" in self.client.must("GET", f"/api/v1/sessions/{sid}/lorebook"))
        status, archive = self.raw("GET", f"/api/v1/sessions/{sid}/export")
        self.check("story package export", status == 200 and archive.startswith(b"PK"), {"bytes": len(archive), "sha256": hashlib.sha256(archive).hexdigest()})
        archive_path = self.path.with_suffix(".tavernpack")
        archive_path.write_bytes(archive)
        self.report["storyPackage"] = archive_path.name
        status, raw = self.raw("POST", "/api/v1/sessions/import", archive, "application/zip")
        if not self.check("story package accepted", status == 201, json.loads(raw)):
            raise RuntimeError("story package rejected")
        imported = self.client.must("GET", "/api/v1/sessions/" + json.loads(raw)["sessionId"])
        self.check("story package import", status == 201 and imported["sessionId"] != sid and imported["state"]["items"] == self.view(session)["state"]["items"])
        self.turn(imported, "我们继续检查航海怀表和旅行茶。", "generation after import")
        bad, _ = self.raw("POST", "/api/v1/sessions/import", archive[:len(archive)//2], "application/zip")
        self.check("damaged package rejected", 400 <= bad < 500)
        self.save()

    def memory(self, session):
        sid, bid, cid = session["sessionId"], session["branch"]["branchId"], session["characterId"]
        endpoint = f"/api/v1/sessions/{sid}/memories?branchId={bid}"
        page = self.client.must("GET", endpoint)
        self.check("memory page and usage", isinstance(page.get("memories"), list) and "total" in page)
        if not page["memories"]:
            self.check("Gemini sourced memory", False, "No memory was extracted from the feature chain")
            return
        memory = page["memories"][0]
        for patch in ({"pinned": True}, {"hidden": True}, {"hidden": False}, {"content": memory["content"] + "（经旅人复核）"}):
            view = self.view(session)
            result = self.client.must("PATCH", f"/api/v1/sessions/{sid}/branches/{bid}/memories/{memory['memoryId']}", {
                **patch, "expectedCharacterId": cid, "expectedHeadId": view["branch"]["headNodeId"], "expectedVersion": view["branch"]["version"], "idempotencyKey": uuid.uuid4().hex})
            page = self.client.must("GET", endpoint)
            self.check("memory " + next(iter(patch)), any(all(m.get(k) == v for k, v in patch.items()) for m in page["memories"]))
        self.client.must("POST", f"/api/v1/sessions/{sid}/branches/{bid}/memories/organize", {"expectedCharacterId": cid})
        self.check("memory organize", isinstance(self.client.must("GET", endpoint)["memories"], list))

    def director(self, session):
        sid, bid, cid = session["sessionId"], session["branch"]["branchId"], session["characterId"]
        base = f"/api/v1/sessions/{sid}/branches/{bid}/director"
        view = self.view(session)
        accepted = self.client.must("POST", base+"/messages", {"expectedCharacterId": cid, "expectedHeadId": view["branch"]["headNodeId"], "expectedDraftVersion": 0, "idempotencyKey": uuid.uuid4().hex, "text": "请依据当前已发生的故事，制定三个简短阶段的大纲：核对物品、前往码头、按约会合。返回包含三个阶段的 plan。"})
        deadline = time.monotonic()+180
        while True:
            result = self.client.must("GET", "/api/v1/director-requests/"+accepted["requestId"])
            if result["status"] != "generating" or time.monotonic() > deadline:
                break
            time.sleep(0.2)
        self.check("Gemini director discussion", result["status"] == "completed" and result.get("draftApplied"), result)
        draft = self.client.must("GET", base)["draft"]
        if not draft or not draft["plan"].get("beats"):
            return
        bad, _ = self.client.request("PUT", base+"/draft", {"expectedCharacterId": cid, "expectedDraftVersion": max(0,draft["version"]-1), "baseRevisionId": draft["baseRevisionId"], "plan": draft["plan"]})
        self.check("director draft conflict", bad == 409)
        for operation in ("activate", "pause", "resume", "skip", "rewind", "complete"):
            view = self.view(session)
            state = self.client.must("GET", base).get("state")
            beat = draft["plan"]["beats"][0]["beatId"] if operation == "rewind" else (state or {}).get("currentBeatId", "")
            result = self.client.must("POST", base+"/commands", {"expectedCharacterId": cid, "expectedHeadId": view["branch"]["headNodeId"], "expectedVersion": view["branch"]["version"], "idempotencyKey": uuid.uuid4().hex, "action": operation, "draftVersion": draft["version"], "beatId": beat})
            self.check("director " + operation, bool(result.get("nodeId")))

    def long_story(self):
        self.e.configure(reflection=True)
        database = f"file:{self.e.directory / 'data/storage.db'}?mode=ro"
        with sqlite3.connect(database, uri=True) as db:
            usage_start = db.execute("SELECT COALESCE(MAX(rowid),0) FROM turn_usage").fetchone()[0]
            summaries_start = db.execute("SELECT COUNT(*) FROM summary_artifacts").fetchone()[0]
        session, _ = self.create("Gemini forty-turn continuity")
        self.report["sessionId"] = session["sessionId"]
        prompts = ["我们沿着海港步道走一小段，观察路边景物。", "我向向导询问灯塔日常的维护工作。", "我们停下听听海浪，聊聊小镇的生活。", "我查看随身物品，继续谨慎前行，不转移也不消耗物品。", "请让向导描述眼前发生的一件小事，并让我回应。"]
        for index in range(40):
            text = prompts[index % len(prompts)]
            if index == 0:
                text = "向导，我把备用钥匙放在蓝色盒子里，并约好明晚在北码头与你会合。请确认并记住这两件事。"
            if index in (9, 19, 29, 39):
                text = "请向导回忆最初约定：备用钥匙放在哪里，我们什么时候到哪里会合？我查看随身物品。"
            turn = self.turn(session, text, f"long story turn {index+1}")
            # Observe each background result before advancing the head. This also
            # prevents a successful reflection from racing the next CAS request.
            time.sleep(0.5)
            deadline = time.monotonic()+120
            while time.monotonic() < deadline:
                with sqlite3.connect(database, uri=True) as db:
                    active = db.execute("SELECT COUNT(*) FROM turn_usage WHERE rowid>? AND outcome='in_flight'", (usage_start,)).fetchone()[0]
                if active == 0:
                    break
                time.sleep(0.25)
            if index in (9, 19, 29, 39):
                prose = "\n".join(b["text"] for b in result_content(turn).get("blocks", []))
                self.check(f"continuity at turn {index+1}", "蓝" in prose and "盒" in prose and "北码头" in prose and "明晚" in prose, prose)
            state = self.view(session)["state"]
            self.check(f"inventory at turn {index+1}", state["items"]["quality_tea"]["quantity"] == 5 and state["items"]["quality_watch"]["quantity"] == 1)
        # Let queued background jobs finish before assigning the next phase.
        time.sleep(2)
        deadline = time.monotonic()+120
        while time.monotonic() < deadline:
            with sqlite3.connect(f"file:{self.e.directory / 'data/storage.db'}?mode=ro", uri=True) as db:
                active = db.execute("SELECT COUNT(*) FROM turn_usage WHERE rowid>? AND outcome='in_flight'", (usage_start,)).fetchone()[0]
            if active == 0:
                break
            time.sleep(1)
        with sqlite3.connect(f"file:{self.e.directory / 'data/storage.db'}?mode=ro", uri=True) as db:
            tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
            counts = {name: db.execute('SELECT COUNT(*) FROM "'+name+'"').fetchone()[0] for name in ("memory_records", "summary_artifacts", "turn_usage") if name in tables}
            tasks = dict(db.execute("SELECT task, COUNT(*) FROM turn_usage WHERE rowid>? AND outcome='completed' GROUP BY task", (usage_start,)))
        self.check("reflection and summary executed", tasks.get("reflection", 0)>0 and tasks.get("summary", 0)>0 and counts.get("summary_artifacts", 0)>summaries_start, {"counts": counts, "tasks": tasks})
        self.save()

    def long_story_tail(self):
        # Reuse the real forty-turn story. Recent corrections can deliberately
        # defer summaries; a small primary window tests pressure compaction.
        original = json.loads((self.e.directory/"long-story.json").read_text(encoding="utf-8"))
        self.report["originalReportSHA256"] = hashlib.sha256((self.e.directory/"long-story.json").read_bytes()).hexdigest()
        self.report["sessionId"] = original["sessionId"]
        session = self.client.must("GET", "/api/v1/sessions/"+original["sessionId"])
        self.e.configure(reflection=True)
        for slot, window, output in (("primary", 8192, 4096), ("reflection", 131072, 4096)):
            self.client.must("PUT", "/api/v1/config/provider", {"slot": slot, "enabled": True, "kind": "openai-chat",
                "baseUrl": f"http://127.0.0.1:{self.e.args.meter_port}/{slot}/v1", "model": "gemini-3.8-flash-high",
                "temperature": 0.4, "contextWindow": window, "maxTokens": output})
        self.report["budgets"] = {"primary": {"contextWindow": 8192, "maxTokens": 4096}, "reflection": {"contextWindow": 131072, "maxTokens": 4096}}
        database = f"file:{self.e.directory / 'data/storage.db'}?mode=ro"
        with sqlite3.connect(database, uri=True) as db:
            previous_summaries = {row[0] for row in db.execute("SELECT s.summary_id FROM summary_artifacts s JOIN plot_nodes p ON p.node_id=s.to_node_id WHERE p.session_id=?", (session["sessionId"],))}
        self.report["existingSummaryIds"] = sorted(previous_summaries)
        new_summaries = []
        start_turn = session["headNode"]["turnNumber"] + 1
        for number in range(start_turn, start_turn+4):
            turn = self.turn(session, "我们暂时停下脚步。请只用一小段旁白描述海风，不加入新设定、约定或物品变化。", f"summary pressure turn {number}")
            time.sleep(0.6)
            deadline = time.monotonic()+150
            while time.monotonic() < deadline:
                with sqlite3.connect(database, uri=True) as db:
                    active = db.execute("SELECT COUNT(*) FROM turn_usage WHERE outcome='in_flight'").fetchone()[0]
                if active == 0: break
                time.sleep(0.25)
            with sqlite3.connect(database, uri=True) as db:
                summaries = db.execute("SELECT s.summary_id,s.from_node_id,s.to_node_id,s.source_hash,s.text FROM summary_artifacts s JOIN plot_nodes p ON p.node_id=s.to_node_id WHERE p.session_id=?", (session["sessionId"],)).fetchall()
            new_summaries = [row for row in summaries if row[0] not in previous_summaries]
            if new_summaries:
                self.report["summaries"] = [dict(zip(("summaryId", "fromNodeId", "toNodeId", "sourceHash", "text"), row)) for row in summaries]
                self.report["newSummaryIds"] = [row[0] for row in new_summaries]
                break
        self.check("new real summary persisted with exact source range", bool(new_summaries), {"newCount": len(new_summaries), "existingCount": len(previous_summaries)})
        for window in (8192, 32768, 131072):
            self.client.must("PUT", "/api/v1/config/provider", {"slot": "primary", "enabled": True, "kind": "openai-chat",
                "baseUrl": f"http://127.0.0.1:{self.e.args.meter_port}/primary/v1", "model": "gemini-3.8-flash-high",
                "temperature": 0.4, "contextWindow": window, "maxTokens": 4096})
            turn = self.turn(session, "请回忆备用钥匙放在哪里，以及最初约定什么时候、到哪里会合。", f"continuity with {window} window")
            prose = "\n".join(block["text"] for block in result_content(turn).get("blocks", []))
            self.check(f"facts with {window} window", "蓝" in prose and "盒" in prose and "北码头" in prose and "明晚" in prose, prose)
            time.sleep(0.6)
            deadline = time.monotonic()+150
            while time.monotonic() < deadline:
                with sqlite3.connect(database, uri=True) as db:
                    if db.execute("SELECT COUNT(*) FROM turn_usage WHERE outcome='in_flight'").fetchone()[0] == 0: break
                time.sleep(0.25)
        self.e.configure()
        self.save()


def run(evaluation, phase):
    suite = Features(evaluation, phase)
    try:
        if phase == "long-story": suite.long_story()
        elif phase == "long-story-tail": suite.long_story_tail()
        else: suite.run()
        suite.report["completed"] = True
        suite.save()
    except Exception as error:
        suite.check("suite completion", False, str(error))
        raise
    if not suite.report["passed"]:
        raise SystemExit(1)


def compatibility(evaluation):
    suite = Features(evaluation, "compatibility")
    try:
        for kind in ("openai-responses", "anthropic-messages"):
            evaluation.client.must("PUT", "/api/v1/config/provider", {"slot": "assist", "enabled": True,
                "kind": kind, "baseUrl": f"http://127.0.0.1:{evaluation.args.meter_port}/assist/v1", "model": "gemini-3.8-flash-high", "maxTokens": 1024, "contextWindow": 32768})
            result = evaluation.client.must("POST", "/api/v1/config/provider/probe", {"slot": "assist", "format": True})
            suite.check(kind + " real compatibility", bool(result.get("ok")), result)
        suite.report["completed"] = True
    finally:
        evaluation.configure()
        suite.save()
    if not suite.report["passed"]:
        raise SystemExit(1)
