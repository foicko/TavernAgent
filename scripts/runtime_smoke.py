"""Exercise a built artifact with fresh data, including forced process recovery.

Only the deterministic local fixture is contacted. User ports/data are unused.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import socket
import sqlite3
import subprocess
import sys
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from config_v2 import configure_slot
from eval_protocol import Client
from mock_model_server import script


def host_target():
    system = {"Windows": "windows", "Darwin": "darwin", "Linux": "linux"}.get(platform.system(), platform.system())
    arch = {"AMD64": "amd64", "x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine(), platform.machine())
    return system + "/" + arch


def smoke(binary, output):
    binary, output = Path(binary).resolve(), Path(output).resolve()
    directory = output.parent / ("runtime-" + str(time.time_ns()))
    directory.mkdir(parents=True)
    report = {"platform": host_target(), "binarySHA256": hashlib.sha256(binary.read_bytes()).hexdigest(), "cases": [], "data": str(directory)}
    def check(name, value):
        report["cases"].append({"name": name, "passed": bool(value)})
        output.write_text(json.dumps(report, indent=2), encoding="utf-8")
        if not value:
            raise AssertionError(name)
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    client = Client(f"http://127.0.0.1:{port}")
    env = {k: v for k, v in os.environ.items() if not k.startswith("TAVERNAGENT_KEY_") and not k.startswith("TAVERNAGENT_EVAL_")}
    command = [str(binary), "-data", str(directory), "-addr", f"127.0.0.1:{port}", "-provider", "mock", "-pin", "482619"]
    log = (directory / "server.log").open("ab", buffering=0)
    startup = None
    flags = 0
    if os.name == "nt":
        # A hidden console lets the test deliver the same Ctrl+C as a user.
        # CREATE_NO_WINDOW has no console to receive control events.
        flags = subprocess.CREATE_NEW_CONSOLE
        startup = subprocess.STARTUPINFO()
        startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
        startup.wShowWindow = 0
    def start():
        process = subprocess.Popen(command, env=env, stdout=log, stderr=log, creationflags=flags, startupinfo=startup)
        deadline = time.monotonic()+25
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise RuntimeError("Artifact exited; see server.log")
            try:
                client.must("GET", "/healthz")
                return process
            except (OSError, ValueError, RuntimeError):
                time.sleep(0.1)
        process.kill()
        process.wait()
        raise RuntimeError("Artifact startup timeout")
    process = None
    fixture_server = None
    release = threading.Event()
    try:
        report["version"] = subprocess.check_output([str(binary), "-version"], env=env, text=True).strip()
        process = start()
        check("empty directory startup", client.must("GET", "/healthz")["ok"])
        with urllib.request.urlopen(client.base_url, timeout=10) as response:
            html = response.read().decode()
        assets = re.findall(r'(?:src|href)="(/assets/[^\"]+)"', html)
        check("embedded entrypoint", bool(assets))
        for asset in assets:
            with urllib.request.urlopen(client.base_url+asset, timeout=10) as response:
                check("embedded "+asset, response.status == 200 and len(response.read()) > 100)
        duplicate = subprocess.run([str(binary), "-data", str(directory), "-addr", "127.0.0.1:0"], env=env, capture_output=True, timeout=15, creationflags=flags, startupinfo=startup)
        check("second process rejected", duplicate.returncode != 0 and process.poll() is None)
        card = json.loads((Path(__file__).parent/"fixtures/quality_story_card.json").read_text(encoding="utf-8"))
        created = client.must("POST", "/api/v1/sessions", {"title": "Artifact smoke", "characterJson": json.dumps(card), "playerName": "旅人", "openingVariantId": "harbor"})
        sid, bid = created["sessionId"], created["branchId"]
        def submit(text, kind="text", **extra):
            view = client.must("GET", "/api/v1/sessions/"+sid)
            return client.must("POST", f"/api/v1/sessions/{sid}/branches/{bid}/turns", {"expectedHeadId": view["branch"]["headNodeId"], "expectedVersion": view["branch"]["version"], "expectedCharacterId": view["characterId"], "idempotencyKey": text, "mode": "structured", "input": {"kind": kind, "text": text, **extra}})
        first = client.wait_turn(submit("喝两份茶", "action", actionRef="drink.tea")["turnId"], 20)
        check("SQLite rule commit", first["status"] == "committed")
        check("inventory persistence", client.must("GET", "/api/v1/sessions/"+sid)["state"]["items"]["quality_tea"]["quantity"] == 3)
        class Fixture(BaseHTTPRequestHandler):
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                frames, _, _ = script(body["messages"])
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()
                try:
                    for index, frame in enumerate(frames):
                        data = {"choices": [{"delta": {"content": json.dumps(frame, ensure_ascii=False)+"\n"}}]}
                        self.wfile.write(("data: "+json.dumps(data, ensure_ascii=False)+"\n\n").encode())
                        self.wfile.flush()
                        if index == 0:
                            release.wait(45)
                    self.wfile.write(b"data: [DONE]\n\n")
                    self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
                    pass
            def log_message(self, *_):
                pass
        fixture_server = ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
        threading.Thread(target=fixture_server.serve_forever, daemon=True).start()
        configure_slot(client, "primary", name="smoke-fixture", kind="openai-chat",
                       base_url=f"http://127.0.0.1:{fixture_server.server_port}/v1",
                       model="deterministic-fixture", max_tokens=1024, context_window=32768)
        pending = submit("[[release:crash]]")
        deadline = time.monotonic()+15
        saved = False
        while time.monotonic() < deadline:
            with sqlite3.connect(f"file:{directory / 'storage.db'}?mode=ro", uri=True) as db:
                saved = db.execute("SELECT COUNT(*) FROM draft_frames f JOIN turn_attempts a ON a.attempt_id=f.attempt_id WHERE a.turn_id=?", (pending["turnId"],)).fetchone()[0] > 0
            if saved:
                break
            time.sleep(0.05)
        check("durable partial frame", saved)
        process.kill()
        process.wait(timeout=15)
        process = start()
        check("forced restart preserves continuation", client.must("GET", "/api/v1/turns/"+pending["turnId"])["status"] == "awaiting_continuation")
        process.kill()
        process.wait(timeout=15)
        process = start()
        check("repeat recovery idempotent", client.must("GET", "/api/v1/turns/"+pending["turnId"])["status"] == "awaiting_continuation")
        release.set()
        client.must("POST", "/api/v1/turns/"+pending["turnId"]+"/continue", {})
        check("continue after recovery", client.wait_turn(pending["turnId"], 25)["status"] == "committed")
        check("no duplicate inventory settlement", client.must("GET", "/api/v1/sessions/"+sid)["state"]["items"]["quality_tea"]["quantity"] == 3)
        with sqlite3.connect(f"file:{directory / 'storage.db'}?mode=ro", uri=True) as db:
            check("SQLite integrity", db.execute("PRAGMA integrity_check").fetchone()[0] == "ok")
            check("branch lock released", db.execute("SELECT active_turn_id FROM branches WHERE branch_id=?", (bid,)).fetchone()[0] == "")
        if os.name == "nt":
            signal_script = """import ctypes,sys
k=ctypes.windll.kernel32
k.FreeConsole()
if not k.AttachConsole(int(sys.argv[1])): raise OSError(ctypes.get_last_error())
k.SetConsoleCtrlHandler(None, True)
if not k.GenerateConsoleCtrlEvent(0, 0): raise OSError(ctypes.get_last_error())
"""
            subprocess.run([sys.executable, "-c", signal_script, str(process.pid)], check=True, timeout=10, creationflags=subprocess.CREATE_NO_WINDOW)
        else:
            process.terminate()
        process.wait(timeout=20)
        check("normal shutdown", process.returncode == 0)
        process = start()
        check("normal shutdown preserves committed story", client.must("GET", "/api/v1/turns/"+pending["turnId"])["status"] == "committed")
        report["passed"] = True
        output.write_text(json.dumps(report, indent=2), encoding="utf-8")
    except Exception as error:
        report.update(passed=False, error=str(error))
        output.write_text(json.dumps(report, indent=2), encoding="utf-8")
        raise
    finally:
        release.set()
        if process is not None and process.poll() is None:
            process.terminate()
            process.wait(timeout=15)
        if fixture_server is not None:
            fixture_server.shutdown()
            fixture_server.server_close()
        log.close()
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(smoke(args.binary, args.out), ensure_ascii=False))
