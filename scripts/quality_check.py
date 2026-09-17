"""真机质量对照：用 chars/ 的真实角色卡跑一轮，打印实际输出与提示词规模。"""
import json
import time
import urllib.error
import urllib.request
from pathlib import Path

API = "http://127.0.0.1:8890"
CARD = Path("data/cardtest/saori-hot-and-dominant-stepaunt-sayuri-s.json")


def req(method, path, body=None, timeout=200):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(API + path, data=data, method=method)
    if data:
        r.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            return resp.status, (json.loads(raw) if raw.strip().startswith(("{", "[")) else raw)
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw


def hash8(s):
    h = 0
    for ch in s:
        h = ((h << 5) - h + ord(ch)) & 0xFFFFFFFF
        if h >= 0x80000000:
            h -= 0x100000000
    return format(abs(h) % 0xFFFFFF, "06x")


payload = json.loads(CARD.read_text(encoding="utf-8"))
d = payload["data"]
desc = "\n\n".join(x for x in [d.get("description"), d.get("personality"), d.get("scenario")] if x)
native = {
    "schemaVersion": 2, "name": d["name"], "description": desc,
    "characters": [{"characterId": "npc_" + hash8(d["name"]), "name": d["name"],
                    "description": desc, "participant": True}],
    "openingVariants": [{"title": "默认开场", "text": d["first_mes"]}],
}
print(f"角色卡：{d['name']}  人设长度={len(desc)} 字")

st, res = req("POST", "/api/v1/sessions", {
    "title": "质量对照", "characterJson": json.dumps(native, ensure_ascii=False),
    "playerName": "林三", "playerRole": "来赴约的旧识"})
print(f"创建会话 HTTP {st}")
sid, bid, head, ver = res["sessionId"], res["branchId"], res["rootNodeId"], res["branchVersion"]

st, turn = req("POST", f"/api/v1/sessions/{sid}/branches/{bid}/turns", {
    "idempotencyKey": f"q-{int(time.time()*1000)}", "expectedHeadId": head, "expectedVersion": ver,
    "input": {"kind": "text", "text": "我推门走进房间，环顾四周，向她点头致意。"}})
print(f"受理回合 HTTP {st}")
tid = turn["turnId"]

for _ in range(200):
    _, tv = req("GET", f"/api/v1/turns/{tid}")
    if tv.get("status") in ("committed", "failed", "cancelled"):
        break
    time.sleep(0.5)

print("=" * 78)
print(f"回合终态：{tv.get('status')}  {tv.get('failureCode') or ''} {str(tv.get('failureMessage') or '')[:120]}")
if tv.get("status") == "committed" and tv.get("resultNode"):
    tc = json.loads(tv["resultNode"]["contentJson"])
    print(f"模式={(tc.get('provenance') or {}).get('mode')}  正文块={len(tc.get('blocks') or [])}  选项={len(tc.get('options') or [])}")
    print("-" * 78)
    for b in tc.get("blocks") or []:
        tag = {"narration": "叙述", "dialogue": "对白", "inner_monologue": "心声"}.get(b["kind"], b["kind"])
        print(f"[{tag}] {b['text']}")
    print("-" * 78)
    for i, o in enumerate(tc.get("options") or [], 1):
        print(f"  选项{i}（{o['intent']}）：{o['text']}")
