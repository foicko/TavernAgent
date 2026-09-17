"""Local OpenAI-compatible fixture for M4 browser and fault checks.

This server is deterministic and never calls an external model.
Markers in the latest user input select faults: [[m4:truncate]],
[[m4:repeat-truncate]], [[m4:invalid]], [[m4:bad-memory]], [[m4:slow]].
"""
import argparse
import json
import re
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

FACT = "灯塔的备用钥匙挂在门后的铜钩上"


def script(messages):
    latest = next((m.get("content", "") for m in reversed(messages) if m.get("role") == "user"), "")
    resume = next((m.get("content", "") for m in reversed(messages)
                   if m.get("role") in ("system", "user") and "（续写）" in m.get("content", "")), "")
    match = re.search(r"从序号 (\d+)", resume)
    seq = int(match.group(1)) if match else 1
    narration = "雨夜的灯火照亮了地图，" + FACT + "。旅人收好行装，准备查看海岸的小路。"
    if resume:
        narration = "雨声渐渐变小，旅人记下了通往海岸的路线。"
    block = {"v": 1, "seq": seq, "type": "block", "kind": "narration", "speakerId": None, "text": narration}
    if "[[m4:invalid]]" in latest:
        block["seq"] = 9
    truncate = "[[m4:repeat-truncate]]" in latest or ("[[m4:truncate]]" in latest and not resume)
    frames = [block]
    if not truncate:
        quote = "不存在的原文证据" if "[[m4:bad-memory]]" in latest else FACT
        frames.append({"v": 1, "seq": seq + 1, "type": "final", "proposals": [
            {"proposalId": "m1", "type": "memory_add", "text": FACT, "memoryKind": "observed",
             "sourceQuote": quote, "evidenceConfidence": "high", "entityIds": ["player"], "participants": ["player"]},
            {"proposalId": "g1", "type": "goal_set", "characterId": "npc_elena", "text": "调查灯塔的夜间信号"},
        ], "options": [{"optionId": "o1", "intent": "clever", "text": "沿着地图寻找灯塔"}]})
    # A continuation's persisted first block contains the fact used as evidence.
    return frames, "length" if truncate else "stop", 0.65 if "[[m4:slow]]" in latest else 0


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"kind":"deterministic-mock","version":"m4-fixture.v1"}')

    def do_POST(self):
        try:
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            frames, finish, delay = script(body.get("messages", []))
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            for frame in frames:
                text = json.dumps(frame, ensure_ascii=False) + "\n"
                for offset in range(0, len(text), 20):
                    chunk = {"choices": [{"delta": {"content": text[offset:offset+20]}, "finish_reason": None}]}
                    self.wfile.write(("data: " + json.dumps(chunk, ensure_ascii=False) + "\n\n").encode())
                    self.wfile.flush()
                    if delay:
                        time.sleep(delay)
            self.wfile.write(("data: " + json.dumps({"choices": [{"delta": {}, "finish_reason": finish}]}) + "\n\n").encode())
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass

    def log_message(self, fmt, *args):
        pass


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=18891)
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    print(f"Deterministic fixture listening on http://127.0.0.1:{args.port}", flush=True)
    server.serve_forever()
