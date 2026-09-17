"""功能架构图的生成回归：图可以重画，但内容不许悄悄少块。

不依赖浏览器（PNG 渲染由 --png 走 Playwright，测试只锁定 SVG 事实）：
每个功能域都必须有入口/服务/落点三行与一段行为约束，右栏五块面板齐全，
且写出的 SVG 里能查到这些文字——防止"改了功能忘了改图"或布局量算把卡片画空。
"""
import sys
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPTS))
import function_architecture_diagram as fd


class FeatureCardTests(unittest.TestCase):
    def test_every_feature_has_three_rows_and_behavior(self):
        features = fd.build_features()
        self.assertEqual([f.key for f in features], [f"F{i}" for i in range(1, 9)])
        for feature in features:
            labels = [label for label, _ in feature.rows]
            self.assertEqual(labels, ["用户入口", "后端服务", "数据落点"], feature.key)
            for _, chips in feature.rows:
                self.assertTrue(chips, f"{feature.key} 有空行")
            self.assertGreater(len(feature.behavior), 30, feature.key)
            self.assertTrue(feature.note.startswith("验收"), feature.key)

    def test_measure_matches_draw_extent(self):
        """量出来的高度必须容得下最后一行行为约束，否则卡片会互相压。"""
        for feature in fd.build_features():
            height = feature.measure(fd.LEFT_W)
            cursor = 34 + 12
            for _, chips in feature.rows:
                rows = fd.Layer._wrap(chips, fd.LEFT_W - fd.CARD_PAD * 2)
                cursor += 17 + len(rows) * 26 + (len(rows) - 1) * 8 + 10
            box_bottom = cursor + 4 + 17 + len(feature.lines) * 17 + 8
            self.assertLessEqual(box_bottom - feature.y, height, feature.key)


class PanelTests(unittest.TestCase):
    def test_right_column_covers_five_panels(self):
        panels = fd.build_panels()
        self.assertEqual(len(panels), 5)
        titles = [p.title for p in panels]
        self.assertEqual(titles[0], "① 一次回合的功能串接")
        self.assertEqual(titles[-1], "⑤ 运行读数（GET /api/status）")
        for panel in panels:
            self.assertTrue(panel.items, panel.title)

    def test_failure_semantics_keep_the_explicit_error_codes(self):
        """显式报错是这批压缩改造的契约，图里必须继续反映它。"""
        body = " ".join(f"{h} {d}" for panel in fd.build_panels() for h, d in panel.items)
        for token in ("409 冲突", "CONTEXT_OVER_BUDGET", "PROTOCOL_INVALID", "PROVIDER_UNAVAILABLE"):
            self.assertIn(token, body)


class RenderTests(unittest.TestCase):
    def test_render_contains_every_card_key_and_behavior(self):
        canvas = fd.Canvas()
        features = fd.build_features()
        panels = fd.build_panels()
        y = 0.0
        for feature in features:
            feature.y = y
            feature.measure(fd.LEFT_W)
            feature.draw(canvas, fd.MARGIN, fd.LEFT_W)
            y += feature.height + 26
        for panel in panels:
            panel.y = y
            panel.measure(fd.RIGHT_W)
            panel.draw(canvas, fd.RIGHT_X, fd.RIGHT_W)
        svg = canvas.render(y, "t", "d")
        for feature in features:
            self.assertIn(feature.title, svg, feature.key)
            self.assertIn(feature.behavior[:18], svg, feature.key)
        self.assertIn("行为与约束", svg)


if __name__ == "__main__":
    unittest.main()
