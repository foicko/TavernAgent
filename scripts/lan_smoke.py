"""Verify pairing through a real private interface using an isolated mock app.

This is a same-host private-network transport check, not a second-device test.
It never changes firewall rules or contacts a model provider.
"""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import socket
import subprocess
import time
import urllib.error
import urllib.request


def smoke(binary, host, output):
    address = ipaddress.ip_address(host)
    if address.version != 4 or not address.is_private or address.is_loopback:
        raise ValueError("--host must be an IPv4 address on this host's private network")
    binary, output = binary.resolve(), output.resolve()
    directory = output.parent / ("lan-runtime-" + str(time.time_ns()))
    directory.mkdir(parents=True)
    with socket.socket() as listener:
        listener.bind((host, 0))
        port = listener.getsockname()[1]
    base = f"http://{host}:{port}"
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    report = {"binarySHA256": hashlib.sha256(binary.read_bytes()).hexdigest(),
              "transport": "same host via private network interface", "host": host,
              "externalDeviceVerified": False, "cases": [], "passed": False}
    def check(name, value):
        report["cases"].append({"name": name, "passed": bool(value)})
        output.write_text(json.dumps(report, indent=2), encoding="utf-8")
        if not value:
            raise AssertionError(name)
    def request(method, path, data=None, token="", origin=None):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        if origin is not None:
            headers["Origin"] = origin
        req = urllib.request.Request(base + path, method=method, headers=headers,
            data=json.dumps(data).encode() if data is not None else None)
        try:
            with opener.open(req, timeout=10) as response:
                return response.status, json.load(response)
        except urllib.error.HTTPError as error:
            return error.code, json.load(error)
    env = {key: value for key, value in os.environ.items()
           if not key.startswith("TAVERNAGENT_KEY_") and not key.startswith("TAVERNAGENT_EVAL_")}
    command = [str(binary), "-data", str(directory), "-addr", host + ":" + str(port),
               "-provider", "mock", "-pin", "482619"]
    log = (directory / "server.log").open("ab", buffering=0)
    process = None
    def start():
        child = subprocess.Popen(command, env=env, stdout=log, stderr=log,
                                 creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
        try:
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                if child.poll() is not None:
                    raise RuntimeError("LAN fixture exited; see server.log")
                try:
                    if request("GET", "/healthz")[0] == 200:
                        return child
                except OSError:
                    pass
                time.sleep(0.1)
            raise RuntimeError("LAN fixture startup timeout")
        except BaseException:
            if child.poll() is None:
                child.kill()
                child.wait(timeout=10)
            raise
    try:
        process = start()
        status, auth = request("GET", "/api/v1/auth/status")
        check("private interface requires pairing", status == 200 and auth.get("required") and not auth.get("authenticated"))
        check("unpaired story read rejected", request("GET", "/api/v1/sessions")[0] == 401)
        check("forged token rejected", request("GET", "/api/v1/sessions", token="invalid-test-token")[0] == 401)
        check("wrong PIN rejected", request("POST", "/api/v1/auth/pair", {"pin": "000000"}, origin=base)[0] == 401)
        check("cross-origin pairing rejected", request("POST", "/api/v1/auth/pair", {"pin": "482619"}, origin="https://untrusted.example")[0] == 403)
        status, paired = request("POST", "/api/v1/auth/pair", {"pin": "482619"}, origin=base)
        token = paired.get("token", "")
        check("same-origin pairing accepted", status == 200 and bool(token))
        check("paired read accepted", request("GET", "/api/v1/sessions", token=token)[0] == 200)
        check("paired cross-origin write rejected", request("POST", "/api/v1/sessions", {}, token, "https://untrusted.example")[0] == 403)
        card = json.loads((Path(__file__).parent / "fixtures/quality_story_card.json").read_text(encoding="utf-8"))
        created, view = request("POST", "/api/v1/sessions", {"title": "LAN isolated acceptance", "characterJson": json.dumps(card), "playerName": "旅人"}, token, base)
        check("paired same-origin story creation", created == 201 and bool(view.get("sessionId")))
        check("paired oversized JSON rejected", request("POST", "/api/v1/sessions", {"title": "x" * (5 * 1024 * 1024)}, token, base)[0] == 413)
        process.terminate()
        process.wait(timeout=15)
        process = start()
        check("restart invalidates old pairing token", request("GET", "/api/v1/sessions", token=token)[0] == 401)
        check("pairing after restart works", request("POST", "/api/v1/auth/pair", {"pin": "482619"}, origin=base)[0] == 200)
        report["passed"] = True
    except Exception as error:
        report["error"] = str(error)
        raise
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            process.wait(timeout=15)
        log.close()
        output.write_text(json.dumps(report, indent=2), encoding="utf-8")
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--host", required=True)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(smoke(args.binary, args.host, args.out), indent=2))
