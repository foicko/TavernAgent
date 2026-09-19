"""Start the deterministic browser-test application with an isolated directory."""
import os
from pathlib import Path
import signal
import subprocess
import time

root = Path(__file__).resolve().parents[1]


def resolve_binary() -> Path:
    """定位浏览器测试用的无头服务二进制。

    不能只认平台默认后缀：文档与 CI 给的构建命令是
    `go build -o build/browser/tavernagent ./cmd/tavernagent`，而 Windows 上
    `-o` 一旦指定了完整名字就不会再补 .exe——只认 tavernagent.exe 会让 Windows
    开发者在跑完这条命令后直接撞 FileNotFoundError（与真实原因毫无关系）。
    两个候选都试，找不到时给出可操作的提示。
    """
    override = os.environ.get("TAVERNAGENT_BROWSER_BINARY")
    if override:
        return Path(override).resolve()
    names = ("tavernagent.exe", "tavernagent") if os.name == "nt" else ("tavernagent", "tavernagent.exe")
    for name in names:
        candidate = root / "build/browser" / name
        if candidate.is_file():
            return candidate.resolve()
    raise SystemExit("找不到浏览器测试用的服务二进制，请先构建：go build -o build/browser/tavernagent ./cmd/tavernagent")


binary = resolve_binary()
directory = root / "output/playwright" / ("data-" + str(time.time_ns()))
directory.mkdir(parents=True)
env = {k: v for k, v in os.environ.items() if not k.startswith("TAVERNAGENT_KEY_") and not k.startswith("TAVERNAGENT_EVAL_")}
process = subprocess.Popen([str(binary), "-addr", "127.0.0.1:18891", "-data", str(directory), "-provider", "mock", "-pin", "482619"], cwd=root, env=env,
    creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
def stop(*_):
    process.terminate()
signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)
try:
    raise SystemExit(process.wait())
finally:
    if process.poll() is None:
        process.terminate()
        process.wait(timeout=15)
