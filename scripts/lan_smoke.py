"""Verify pairing through a real private interface using an isolated mock app.

This is a same-host private-network transport check, not a second-device test.
It never changes firewall rules or contacts a model provider.

With --tls it repeats the pairing flow over the self-signed HTTPS listener and adds
the transport-specific checks: certificate persistence/reuse, and that plaintext
HTTP is refused on the same port.
"""
import argparse
import hashlib
import http.client
import ipaddress
import json
import os
from pathlib import Path
import socket
import ssl
import subprocess
import time
import urllib.error
import urllib.request


def plaintext_refused(host, port):
    """明文 HTTP 打到 TLS 端口必须失败：能通就说明加密只是"看起来开着"。"""
    probe = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with probe.open(urllib.request.Request(f"http://{host}:{port}/healthz"), timeout=5) as response:
            return response.status != 200
    except Exception:
        return True


def certificate_covers(certificate, host):
    """证书 SAN 必须覆盖实际绑定的地址，否则设备访问会报证书不匹配。

    用标准库自带的 PEM 解码（ssl 的测试辅助函数）而不是引入 cryptography：
    本仓库的脚本层刻意保持零第三方依赖。
    """
    try:
        decoded = ssl._ssl._test_decode_cert(str(certificate))
    except Exception:
        return False
    return any(kind == "IP Address" and value == host
               for kind, value in decoded.get("subjectAltName", ()))


def smoke(binary, host, output, tls=False):
    address = ipaddress.ip_address(host)
    if address.version != 4 or not address.is_private or address.is_loopback:
        raise ValueError("--host must be an IPv4 address on this host's private network")
    binary, output = binary.resolve(), output.resolve()
    directory = output.parent / ("lan-runtime-" + str(time.time_ns()))
    directory.mkdir(parents=True)
    with socket.socket() as listener:
        listener.bind((host, 0))
        port = listener.getsockname()[1]
    scheme = "https" if tls else "http"
    base = f"{scheme}://{host}:{port}"
    # 自签证书在"用户还没把它装进信任库"时必然校验失败——这正是被测对象的前置状态，
    # 所以这里显式跳过校验；信任动作由人在设备上完成，脚本不代替。
    handlers = [urllib.request.ProxyHandler({})]
    if tls:
        handlers.append(urllib.request.HTTPSHandler(context=ssl._create_unverified_context()))
    opener = urllib.request.build_opener(*handlers)
    report = {"binarySHA256": hashlib.sha256(binary.read_bytes()).hexdigest(),
              "transport": ("TLS over " if tls else "") + "same host via private network interface",
              "host": host, "scheme": scheme,
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
    def oversized_rejected(token):
        """超大请求体必须不被受理。

        建会话的额度与角色卡导入对齐（8MB：卡片要带着内嵌头像与世界书一起提交），
        所以探针必须超过 8MB。这类"客户端还在写、服务端已经决定拒绝"的请求，观测到的
        往往是连接被重置而不是 413 本身——因此断言的是"没有被受理"，并顺带确认服务
        仍然健康（否则一次网络故障会被误判成"拒绝成功"）。
        """
        try:
            return request("POST", "/api/v1/sessions", {"title": "x" * (9 * 1024 * 1024)}, token, base)[0] == 413
        except (urllib.error.URLError, http.client.HTTPException, OSError):
            return request("GET", "/api/v1/sessions", token=token)[0] == 200

    env = {key: value for key, value in os.environ.items()
           if not key.startswith("TAVERNAGENT_KEY_") and not key.startswith("TAVERNAGENT_EVAL_")}
    command = [str(binary), "-data", str(directory), "-addr", host + ":" + str(port),
               "-provider", "mock", "-pin", "482619"]
    if tls:
        command += ["-tls", "auto"]
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
        check("paired oversized JSON rejected", oversized_rejected(token))
        certificate = directory / "tls" / "server.crt"
        fingerprint_before = hashlib.sha256(certificate.read_bytes()).hexdigest() if certificate.exists() else ""
        process.terminate()
        process.wait(timeout=15)
        process = start()
        check("restart invalidates old pairing token", request("GET", "/api/v1/sessions", token=token)[0] == 401)
        check("pairing after restart works", request("POST", "/api/v1/auth/pair", {"pin": "482619"}, origin=base)[0] == 200)
        if tls:
            check("self-signed certificate persisted under the data directory", bool(fingerprint_before))
            check("certificate reused across restart (fingerprint stable)",
                  certificate.exists() and hashlib.sha256(certificate.read_bytes()).hexdigest() == fingerprint_before)
            check("plaintext HTTP refused on the TLS port", plaintext_refused(host, port))
            check("certificate covers the bound private address", certificate_covers(certificate, host))
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
    parser.add_argument("--tls", action="store_true",
                        help="以 -tls auto 启动被测实例，在自签 HTTPS 上重跑配对流程")
    args = parser.parse_args()
    print(json.dumps(smoke(args.binary, args.host, args.out, tls=args.tls), indent=2))
