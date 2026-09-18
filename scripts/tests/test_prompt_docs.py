"""提示词文档的同步回归：docs/PROMPTS.md 必须与源码逐字一致。

没有这道测试，提示词文档必然在几次迭代后失真——而"看上去对、其实和线上跑的不一样"
的文档比没有文档更危险。
"""
import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import prompt_docs


class PromptDocsTests(unittest.TestCase):
    def test_document_is_in_sync_with_source(self):
        self.assertEqual(
            prompt_docs.DOC.read_text(encoding="utf-8"),
            prompt_docs.render_doc(),
            "docs/PROMPTS.md 与源码不同步：请运行 python scripts/prompt_docs.py",
        )

    def test_every_registered_prompt_is_extractable(self):
        for _, rel, const, _ in prompt_docs.CONSTANTS:
            with self.subTest(const=const):
                self.assertTrue(prompt_docs.extract(rel, const).strip(), f"{const} 提取为空")


if __name__ == "__main__":
    unittest.main()
