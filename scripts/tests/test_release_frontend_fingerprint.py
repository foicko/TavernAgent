"""前端产物指纹的回归：dist 是生成物、不入库，指纹必须直接来自磁盘。

历史缺陷：frontendSHA256 取自 git 索引里 web/dist/ 前缀的子集。
一旦 dist 不再入库（.gitignore），那样算出来的就是空集合的哈希——字段还在，
但不再代表任何东西，发布台账会静默失真。
"""
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import release


class FrontendFingerprintTests(unittest.TestCase):
    def test_fingerprint_follows_the_built_directory(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "web" / "dist" / "assets").mkdir(parents=True)
            (root / "web" / "dist" / "index.html").write_text("<html>", encoding="utf-8")
            (root / "web" / "dist" / "assets" / "index-abc.js").write_text("console.log(1)", encoding="utf-8")
            with patch.object(release, "ROOT", root):
                files = release.frontend_files()
                manifest = {path.relative_to(root).as_posix(): release.digest(path) for path in files}
            self.assertEqual(set(manifest), {"web/dist/index.html", "web/dist/assets/index-abc.js"})

    def test_missing_dist_fails_loudly(self):
        with tempfile.TemporaryDirectory() as directory:
            with patch.object(release, "ROOT", Path(directory)):
                with self.assertRaises(SystemExit):
                    release.frontend_files()


if __name__ == "__main__":
    unittest.main()
