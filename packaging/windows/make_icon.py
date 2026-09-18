"""生成 Windows 应用图标（.ico）。

母题取自项目现有的 web/public/assets/original/custom.svg：深板岩圆角"玻璃"底、
浅色人像剪影与细边框。仓库只提交 .ico（release_audit 仅对 .png/.jpg 等位图判违规），
预览 PNG 写到 output/（已忽略）。

用法：
    python packaging/windows/make_icon.py            # 只生成 icon.ico
    python packaging/windows/make_icon.py --preview  # 额外输出 output/icon-preview.png
"""
from __future__ import annotations

import argparse
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[2]
ICO = ROOT / "packaging/windows/icon.ico"
PREVIEW = ROOT / "output/icon-preview.png"

S = 1024  # 高分辨率超采样，再交给 Pillow 生成各尺寸

SLATE_TOP = (0x3F, 0x4A, 0x5A)
SLATE_BOTTOM = (0x23, 0x2A, 0x36)
LIGHT = (0xB8, 0xC4, 0xD1)


def lerp(a: tuple[int, int, int], b: tuple[int, int, int], t: float) -> tuple[int, int, int]:
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))  # type: ignore[return-value]


def render() -> Image.Image:
    content = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    draw = ImageDraw.Draw(content)

    # 竖向渐变底
    for y in range(S):
        draw.line([(0, y), (S, y)], fill=lerp(SLATE_TOP, SLATE_BOTTOM, y / (S - 1)) + (255,))

    # 人像剪影：头（实心圆）+ 肩（实心穹顶）。
    # 不画高光/内框/细节：16px 下只有“大而深的对比”能活下来。
    head_r = 150
    draw.ellipse([S // 2 - head_r, 400 - head_r, S // 2 + head_r, 400 + head_r], fill=LIGHT + (255,))
    draw.pieslice([300, 516, 724, 916], start=180, end=360, fill=LIGHT + (255,))

    # 圆角裁切
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=232, fill=255)
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.paste(content, (0, 0), mask)
    return out


def preview(base: Image.Image) -> None:
    sizes = [256, 48, 32, 16]
    tiles = [base.resize((s, s), Image.Resampling.LANCZOS) for s in sizes]
    pad, gap = 24, 28
    width = pad * 2 + sum(t.width for t in tiles) + gap * (len(tiles) - 1)
    height = pad * 2 + max(t.height for t in tiles)
    canvas = Image.new("RGBA", (width, height), (0xF9, 0xF8, 0xF5, 255))
    x = pad
    for tile in tiles:
        canvas.alpha_composite(tile, (x, pad + (max(t.height for t in tiles) - tile.height) // 2))
        x += tile.width + gap
    PREVIEW.parent.mkdir(parents=True, exist_ok=True)
    canvas.save(PREVIEW)
    print(f"preview: {PREVIEW.relative_to(ROOT)}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--preview", action="store_true", help="额外输出 output/icon-preview.png")
    args = parser.parse_args()

    base = render()
    ICO.parent.mkdir(parents=True, exist_ok=True)
    base.resize((256, 256), Image.Resampling.LANCZOS).save(
        ICO, format="ICO", sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (24, 24), (16, 16)]
    )
    print(f"icon: {ICO.relative_to(ROOT)}")
    if args.preview:
        preview(base)


if __name__ == "__main__":
    main()
