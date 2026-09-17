"""Start the deterministic browser-test application with an isolated directory."""
import os
from pathlib import Path
import signal
import subprocess
import time

root = Path(__file__).resolve().parents[1]
binary = Path(os.environ.get("TAVERNAGENT_BROWSER_BINARY", str(root / "build/browser" / ("tavernagent.exe" if os.name == "nt" else "tavernagent")))).resolve()
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
