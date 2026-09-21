"""Local inference gateway with an immutable, crash-safe 300-attempt budget.

All three application slots must use /<slot>/v1 on this server. Credentials
are read only from TAVERNAGENT_EVAL_GATEWAY_KEY and TAVERNAGENT_EVAL_TOKEN.
The SQLite ledger reserves before upstream I/O; abandoned reservations count.
No retries, redirects, or alternate models are performed by this gateway.
"""
import argparse
import hashlib
import http.client
import json
import os
import sqlite3
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

MODEL = os.environ.get("TAVERNAGENT_EVAL_MODEL", "gemini-3.8-flash-high")
# 模型标签可以改（--model / TAVERNAGENT_EVAL_MODEL），默认值保持不变：
# 已有台账里记着旧标签，改默认值会让"这一轮到底跑了哪个模型"变成需要考古的问题。


def utc():
    return datetime.now(timezone.utc).isoformat()


class BudgetExhausted(Exception):
    pass


class Ledger:
    def __init__(self, directory, limit=300, model=MODEL):
        if not 1 <= limit <= 300:
            raise ValueError("Inference budget must be between 1 and 300")
        self.model = model
        self.directory = Path(directory).resolve()
        self.directory.mkdir(parents=True, exist_ok=True)
        self.path = self.directory / "calls.sqlite3"
        with self.connect() as db:
            db.executescript("""
                CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
                CREATE TABLE IF NOT EXISTS calls(
                  id INTEGER PRIMARY KEY AUTOINCREMENT, started TEXT NOT NULL,
                  phase TEXT NOT NULL, slot TEXT NOT NULL, model TEXT NOT NULL,
                  protocol TEXT NOT NULL, request_sha256 TEXT NOT NULL,
                  outcome TEXT NOT NULL DEFAULT 'reserved', http_status INTEGER,
                  latency_ms REAL, first_byte_ms REAL, response_bytes INTEGER,
                  response_sha256 TEXT, completed TEXT);
            """)
            db.execute("BEGIN IMMEDIATE")
            row = db.execute("SELECT value FROM metadata WHERE key='limit'").fetchone()
            if row and int(row[0]) != limit:
                raise ValueError("Existing budget is immutable; resume with its original limit")
            db.execute("INSERT OR IGNORE INTO metadata VALUES('limit',?)", (str(limit),))
            db.execute("INSERT OR IGNORE INTO metadata VALUES('phase','preflight')")

    @contextmanager
    def connect(self):
        db = sqlite3.connect(self.path, timeout=30)
        try:
            db.execute("PRAGMA journal_mode=WAL")
            db.execute("PRAGMA synchronous=FULL")
            with db:
                yield db
        finally:
            db.close()

    def reserve(self, slot, protocol, payload):
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            limit = int(db.execute("SELECT value FROM metadata WHERE key='limit'").fetchone()[0])
            count = db.execute("SELECT COUNT(*) FROM calls").fetchone()[0]
            if count >= limit:
                raise BudgetExhausted("No inference was sent: the persisted budget is exhausted")
            phase = db.execute("SELECT value FROM metadata WHERE key='phase'").fetchone()[0]
            cursor = db.execute("INSERT INTO calls(started,phase,slot,model,protocol,request_sha256) VALUES(?,?,?,?,?,?)",
                                (utc(), phase, slot, self.model, protocol, hashlib.sha256(payload).hexdigest()))
            return cursor.lastrowid

    def finish(self, call_id, outcome, status, elapsed, first, size, digest):
        with self.connect() as db:
            db.execute("UPDATE calls SET outcome=?,http_status=?,latency_ms=?,first_byte_ms=?,response_bytes=?,response_sha256=?,completed=? WHERE id=?",
                       (outcome, status, elapsed, first, size, digest, utc(), call_id))

    def phase(self, value):
        if value not in {"preflight", "protocol", "features", "long_story", "retest"}:
            raise ValueError("Unknown evaluation phase")
        with self.connect() as db:
            db.execute("UPDATE metadata SET value=? WHERE key='phase'", (value,))

    def snapshot(self):
        with self.connect() as db:
            db.row_factory = sqlite3.Row
            limit = int(db.execute("SELECT value FROM metadata WHERE key='limit'").fetchone()[0])
            calls = [dict(row) for row in db.execute("SELECT * FROM calls ORDER BY id")]
        return {"limit": limit, "used": len(calls), "remaining": limit-len(calls), "model": self.model, "calls": calls}


class MeterServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, port, ledger, upstream, gateway_key, token, model=MODEL):
        super().__init__(("127.0.0.1", port), Handler)
        self.ledger, self.upstream = ledger, urlsplit(upstream.rstrip("/"))
        if self.upstream.scheme not in {"http", "https"} or not self.upstream.hostname or self.upstream.username:
            raise ValueError("Invalid upstream URL")
        self.gateway_key, self.token = gateway_key, token
        # 只放行被批准的模型：网关的预算与台账都按这个标签记账，
        # 放行任意模型等于把"这一轮花了多少"变成不可对账的数字。
        self.model = model


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def authorized(self):
        import hmac
        return (hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + self.server.token)
                or hmac.compare_digest(self.headers.get("x-api-key", ""), self.server.token))

    def reply(self, status, value):
        body = json.dumps(value, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)
        self.close_connection = True

    def do_GET(self):
        if not self.authorized():
            self.reply(401, {"error": {"code": "UNAUTHORIZED"}})
            return
        if self.path == "/ledger":
            self.reply(200, self.server.ledger.snapshot())
        else:
            self.reply(404, {"error": {"code": "NOT_FOUND"}})

    def do_POST(self):
        if not self.authorized():
            self.reply(401, {"error": {"code": "UNAUTHORIZED"}})
            return
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if not 0 < size <= 16*1024*1024:
                self.reply(413, {"error": {"code": "REQUEST_TOO_LARGE"}})
                return
            payload = self.rfile.read(size)
            body = json.loads(payload)
            if self.path == "/control/phase":
                self.server.ledger.phase(body["phase"])
                self.reply(200, {"phase": body["phase"]})
                return
            parts = self.path.strip("/").split("/", 2)
            if len(parts) != 3 or parts[0] not in {"primary", "assist", "reflection"} or parts[1] != "v1" or parts[2] not in {"chat/completions", "responses", "messages"}:
                self.reply(404, {"error": {"code": "INVALID_INFERENCE_PATH"}})
                return
            slot, _, protocol = parts
            if body.get("model") != self.server.model:
                self.reply(422, {"error": {"code": "MODEL_MISMATCH", "message": "Only the approved Gemini model is allowed"}})
                return
            call_id = self.server.ledger.reserve(slot, protocol, payload)
        except BudgetExhausted as error:
            self.reply(429, {"error": {"code": "EVALUATION_BUDGET_EXHAUSTED", "message": str(error)}})
            return
        except (ValueError, KeyError, TypeError):
            self.reply(400, {"error": {"code": "INVALID_REQUEST"}})
            return

        started = time.monotonic()
        first, status, size = None, None, 0
        digest = hashlib.sha256()
        upstream = self.server.upstream
        connection_type = http.client.HTTPSConnection if upstream.scheme == "https" else http.client.HTTPConnection
        connection = connection_type(upstream.hostname, upstream.port, timeout=180)
        outcome, headers_sent = "transport_error", False
        try:
            # Evidence is synthetic evaluation data; authorization headers are never saved.
            (self.server.ledger.directory / f"request-{call_id:03d}.json").write_bytes(payload)
            headers = {"Authorization": "Bearer " + self.server.gateway_key, "Content-Type": "application/json"}
            if protocol == "messages":
                headers.update({"x-api-key": self.server.gateway_key, "anthropic-version": "2023-06-01"})
            connection.request("POST", upstream.path + "/" + protocol, body=payload, headers=headers)
            response = connection.getresponse()
            status = response.status
            if not 200 <= status < 300:
                # A vendor error may echo credentials; keep only the HTTP status.
                outcome = "upstream_error"
                self.reply(status, {"error": {"code": "UPSTREAM_ERROR", "message": f"Gemini gateway returned HTTP {status}"}})
                return
            self.send_response(status)
            self.send_header("Content-Type", response.getheader("Content-Type", "text/event-stream"))
            self.send_header("Connection", "close")
            self.end_headers()
            headers_sent = True
            self.close_connection = True
            with (self.server.ledger.directory / f"response-{call_id:03d}.sse").open("wb") as evidence:
                while True:
                    chunk = response.read1(65536)
                    if not chunk:
                        break
                    if first is None:
                        first = (time.monotonic()-started)*1000
                    digest.update(chunk)
                    size += len(chunk)
                    evidence.write(chunk)
                    self.wfile.write(chunk)
                    self.wfile.flush()
            outcome = "completed"
        except (BrokenPipeError, ConnectionResetError):
            outcome = "client_disconnected"
        except (OSError, http.client.HTTPException):
            if not headers_sent:
                try:
                    self.reply(502, {"error": {"code": "UPSTREAM_CONNECTION_FAILED"}})
                except OSError:
                    pass
        finally:
            connection.close()
            self.server.ledger.finish(call_id, outcome, status, (time.monotonic()-started)*1000, first, size, digest.hexdigest())
            print(json.dumps({"call": call_id, "slot": slot, "outcome": outcome, "httpStatus": status}), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, required=True)
    parser.add_argument("--upstream", required=True)
    parser.add_argument("--port", type=int, default=18892)
    parser.add_argument("--limit", type=int, default=300)
    parser.add_argument("--model", default=MODEL,
                        help="只放行的模型标签（默认取 TAVERNAGENT_EVAL_MODEL，再退到内置默认值）")
    args = parser.parse_args()
    key, token = os.environ.get("TAVERNAGENT_EVAL_GATEWAY_KEY"), os.environ.get("TAVERNAGENT_EVAL_TOKEN")
    if not key or not token:
        parser.error("Set both evaluation credential environment variables")
    ledger = Ledger(args.directory, args.limit, args.model)
    server = MeterServer(args.port, ledger, args.upstream, key, token, args.model)
    print(json.dumps({"port": args.port, "used": ledger.snapshot()["used"], "limit": args.limit, "model": args.model}), flush=True)
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
