"""Build pinned frontend + embedded binaries and archive five release targets.

This command produces local files only. It never publishes a GitHub Release.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import time
import zipfile

from runtime_smoke import host_target, smoke

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ("windows/amd64", "darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64")


def run(command, cwd=ROOT, env=None):
    executable = shutil.which(command[0])
    if executable is None:
        raise RuntimeError("Required command missing: " + command[0])
    subprocess.run([executable, *command[1:]], cwd=cwd, env=env, check=True)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sources():
    names = subprocess.check_output(["git", "ls-files", "-co", "--exclude-standard", "-z"], cwd=ROOT).decode().split("\0")
    return sorted({name for name in names if name and (ROOT/name).is_file()})


def frontend_files():
    """嵌入二进制的正是 web/dist 目录本身：指纹直接遍历磁盘，不依赖 git 索引
    （dist 是生成物、不入库，见 .gitignore）。"""
    dist = ROOT/"web"/"dist"
    if not dist.is_dir():
        raise SystemExit("缺少 web/dist：前端产物不入库，请先执行 pnpm build")
    return sorted(path for path in dist.rglob("*") if path.is_file())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="0.1.0-rc.1")
    parser.add_argument("--target", choices=("all", *TARGETS), default="all")
    parser.add_argument("--out", type=Path, default=ROOT / "build/release")
    parser.add_argument("--skip-install", action="store_true")
    parser.add_argument("--skip-checks", action="store_true", help="Only after corresponding checks ran separately")
    parser.add_argument("--smoke", action="store_true", help="Run the archived artifact on this host; mismatched targets are refused")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9A-Za-z.+-]+", args.version):
        parser.error("invalid version")
    release_notes = ROOT/"docs"/f"RELEASE_NOTES_{args.version}.md"
    targets = TARGETS if args.target == "all" else (args.target,)
    if args.smoke and any(target != host_target() for target in targets):
        parser.error("--smoke requires exactly this host's target: " + host_target())
    if not args.skip_install:
        run(["pnpm", "install", "--frozen-lockfile"], ROOT/"web")
    # 前端产物必须在**任何 Go 命令之前**生成：web/embed.go 用 `//go:embed all:dist`
    # 把界面嵌进二进制，干净检出（dist 是生成物、不入库）下 go test / go vet 会直接
    # 报 “pattern all:dist: no matching files found”。此前这一段排在检查之后，于是
    # 每个平台的矩阵作业都在第一步就挂掉——而它在本地永远不会重现，因为本地跑过
    # 一次前端构建后 dist 就一直留在磁盘上了。
    run(["pnpm", "build"], ROOT/"web")
    if not args.skip_checks:
        for command in (["pnpm", "typecheck"], ["pnpm", "lint"], ["pnpm", "test"]):
            run(command, ROOT/"web")
        run(["go", "test", "-count=1", "./..."])
        run(["go", "vet", "./..."])
    files = sources()
    manifest = {name: digest(ROOT/name) for name in files}
    source_hash = hashlib.sha256(json.dumps(manifest, sort_keys=True).encode()).hexdigest()
    front_manifest = {path.relative_to(ROOT).as_posix(): digest(path) for path in frontend_files()}
    front_hash = hashlib.sha256(json.dumps(front_manifest, sort_keys=True).encode()).hexdigest()
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    if subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT).strip():
        commit += "-dirty"
    built_at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    output = args.out.resolve()
    output.mkdir(parents=True, exist_ok=True)
    metadata = {"version": args.version, "commit": commit, "builtAt": built_at, "sourceSHA256": source_hash,
                "frontendSHA256": front_hash, "frontendFiles": len(front_manifest),
                "go": subprocess.check_output(["go", "version"], cwd=ROOT, text=True).strip()}
    archives = []
    for target in targets:
        system, arch = target.split("/")
        name = f"tavernagent-{args.version}-{system}-{arch}"
        folder = output / name
        folder.mkdir(exist_ok=True)
        executable = folder / ("tavernagent.exe" if system == "windows" else "tavernagent")
        environment = {**os.environ, "GOOS": system, "GOARCH": arch, "CGO_ENABLED": "0"}
        run(["go", "build", "-trimpath", "-ldflags", f"-s -w -X main.appVersion=tavernagent/{args.version} -X main.buildCommit={commit} -X main.buildTime={built_at}", "-o", str(executable), "./cmd/tavernagent"], env=environment)
        info = {**metadata, "target": target, "binarySHA256": digest(executable),
                "runtimeEvidence": "See the matching smoke report; cross-compilation alone does not verify runtime behavior."}
        for source in ("LICENSE", "THIRD_PARTY_NOTICES.md", "docs/OPERATIONS.md", "docs/ASSET_LICENSES.md"):
            shutil.copyfile(ROOT/source, folder/Path(source).name)
        notices = folder/"THIRD_PARTY_NOTICES.md"
        notices.write_text(notices.read_text(encoding="utf-8").replace("](docs/ASSET_LICENSES.md)", "](ASSET_LICENSES.md)"), encoding="utf-8")
        if release_notes.is_file():
            shutil.copyfile(release_notes, folder/"RELEASE_NOTES.md")
        shutil.copytree(ROOT/"LICENSES", folder/"LICENSES", dirs_exist_ok=True)
        (folder/"START.txt").write_text("TavernAgent\n\nWindows: .\\tavernagent.exe\nmacOS/Linux: ./tavernagent\nBrowser: http://127.0.0.1:8890\nData: ./data (use -data to choose another directory)\nRead OPERATIONS.md before LAN access or upgrading.\n", encoding="utf-8")
        (folder/"BUILD-INFO.json").write_text(json.dumps(info, indent=2)+"\n", encoding="utf-8")
        if system == "windows":
            archive = output/(name+".zip")
            with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as stream:
                for file in sorted(folder.rglob("*")):
                    if file.is_file(): stream.write(file, file.relative_to(output).as_posix())
        else:
            archive = output/(name+".tar.gz")
            with tarfile.open(archive, "w:gz") as stream:
                for file in sorted(folder.rglob("*")):
                    if not file.is_file(): continue
                    entry = stream.gettarinfo(str(file), file.relative_to(output).as_posix())
                    entry.uid = entry.gid = 0
                    entry.uname = entry.gname = ""
                    entry.mode = 0o755 if file == executable else 0o644
                    with file.open("rb") as contents: stream.addfile(entry, contents)
        archives.append(archive)
        if args.smoke:
            extracted = output/("verify-"+name+"-"+str(time.time_ns()))
            extracted.mkdir()
            if system == "windows":
                with zipfile.ZipFile(archive) as stream: stream.extractall(extracted)
            else:
                with tarfile.open(archive) as stream: stream.extractall(extracted, filter="data")
            smoke(extracted/name/executable.name, output/("smoke-"+system+"-"+arch+".json"))
    source_zip = output / "source-clean.zip"
    with zipfile.ZipFile(source_zip, "w", compression=zipfile.ZIP_DEFLATED) as stream:
        for name in files: stream.write(ROOT/name, name)
        stream.writestr("SOURCE-FINGERPRINT.json", json.dumps({**metadata, "files": manifest}, indent=2))
    archives.append(source_zip)
    (output/"SHA256SUMS.txt").write_text("".join(digest(archive)+"  "+archive.name+"\n" for archive in archives), encoding="utf-8")
    (output/"source-manifest.json").write_text(json.dumps({**metadata, "files": manifest}, indent=2), encoding="utf-8")
    print(json.dumps({"output": str(output), "artifacts": [p.name for p in archives], **metadata}, ensure_ascii=False))


if __name__ == "__main__":
    main()
