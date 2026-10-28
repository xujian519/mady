#!/usr/bin/env python3
"""附图栅格化：SVG 源件（首选）或 `<附图>_vN.html`（回退）→ 300 DPI 等效 PNG。

## 首选：直接栅格化 SVG 源件

每份申请文件都带矢量附图源件。直接 SVG → PNG 是**最干净**的路径：

* 无需裁切——SVG 里没有事务所抬头、没有页眉横线、没有页码；
* 图号「图N」通常已画在 SVG 内，不会与后加的文本图号重复；
* 尺寸由 SVG 的 `width`/`height` 决定，输出确定性高。

两种指明源件的方式：``--svg-file`` 按给定顺序逐图出件（调用方已排好图序时用这个），
``--svg-dir`` 按 ``figN.svg`` 的编号出件。两者互斥。

## 回退：只有 HTML 附图件时

渲染链 `HTML --(headless Chrome)--> PDF --(裁切)--> PNG`。裁切必须**同时排除页眉/页脚
区域的图形与文字**：品牌页眉下方的横线是一条"宽度 > 2"的矢量路径，只按 text 过滤会让它
把包围盒一路顶到页顶，成品附图上会带上事务所抬头。该路径另需 pymupdf。

用法::

    python3 render_figures.py --svg-file a.svg --svg-file b.svg --out-dir figures/
    python3 render_figures.py --svg-dir 附图/ --out-dir figures/
    python3 render_figures.py --html 附图_v6.html --out-dir figures/

标准输出是写出文件的 JSON 列表。
"""

from __future__ import annotations

import argparse
import json
import math
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

CHROME_CANDIDATES = (
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    r"C:\Program Files\Google\Chrome\Application\chrome.exe",
    r"C:\Program Files (x86)\Google\Chrome\Application\chrome.exe",
    "google-chrome",
    "chromium",
    "chrome.exe",
    "chromium.exe",
)

HEADER_BELOW_PT = 60.0   # HTML 回退路径：其上为页眉区（品牌条 + 其下横线）
FOOTER_ABOVE_PT = 800.0  # HTML 回退路径：其下为页脚区（页码）
FIGURE_CAPTION = re.compile(r"^图\s*\d+$")

SVG_OPEN = re.compile(r"<svg\b[^>]*>", re.IGNORECASE)
SVG_ATTRIBUTE = re.compile(r"""([:\w-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')""")
CSS_LENGTH = re.compile(r"^\s*(\d+(?:\.\d+)?)\s*([a-z]*)\s*$", re.IGNORECASE)
# CSS 绝对单位到 96 dpi 像素的换算；无单位按 px。em/%/rem 依赖外部上下文，按缺失处理。
CSS_UNIT_PX = {
    "": 1.0,
    "px": 1.0,
    "in": 96.0,
    "cm": 96.0 / 2.54,
    "mm": 96.0 / 25.4,
    "pt": 96.0 / 72.0,
    "pc": 16.0,
}


def find_chrome(explicit: str | None = None) -> str:
    """定位 Chrome/Chromium 可执行文件；显式路径不存在时不回落自动探测。"""
    if explicit:
        if shutil.which(explicit) or Path(explicit).exists():
            return explicit
        raise SystemExit(f"指定的 Chrome 不存在：{explicit}")
    for c in CHROME_CANDIDATES:
        found = shutil.which(c) or (c if Path(c).exists() else None)
        if found:
            return found
    raise SystemExit("未找到 Chrome/Chromium；请用 --chrome 指定可执行文件")


def _figure_sort_key(p: Path) -> int:
    m = re.search(r"(\d+)", p.stem)
    return int(m.group(1)) if m else 0


def _svg_pixel_size(src: Path) -> tuple[int, int]:
    """把 SVG 根元素的物理 width/height 换算成 96 dpi 下的截图像素。"""
    open_tag = SVG_OPEN.search(src.read_text(encoding="utf-8"))
    attributes: dict[str, str] = {}
    if open_tag is not None:
        for match in SVG_ATTRIBUTE.finditer(open_tag.group(0)):
            attributes[match.group(1).lower()] = (
                match.group(2) if match.group(2) is not None else match.group(3)
            )
    pixels = []
    for name in ("width", "height"):
        length = CSS_LENGTH.match(attributes.get(name, ""))
        if length is None or length.group(2).lower() not in CSS_UNIT_PX:
            raise SystemExit(
                f"{src.name} 的 width/height 无法换算成像素"
                "（支持 mm/cm/in/pt/pc/px 与纯数字），无法确定输出尺寸"
            )
        factor = CSS_UNIT_PX[length.group(2).lower()]
        pixels.append(max(1, math.ceil(float(length.group(1)) * factor)))
    return pixels[0], pixels[1]


def _rasterize(src: Path, target: Path, chrome: str, scale: int) -> None:
    """按 SVG 自身的 width/height × scale 栅格化一张图。"""
    w, h = _svg_pixel_size(src)
    subprocess.run(
        [chrome, "--headless", "--disable-gpu", "--hide-scrollbars",
         "--default-background-color=FFFFFF",
         f"--force-device-scale-factor={scale}",
         f"--window-size={w},{h}", f"--screenshot={target}", src.resolve().as_uri()],
        check=True, capture_output=True,
    )


def render_files(sources: list[Path], out_dir: Path, chrome: str, scale: int) -> list[str]:
    """按给定顺序逐图出件，输出 fig1.png … figN.png。"""
    missing = [str(p) for p in sources if not p.is_file()]
    if missing:
        raise SystemExit(f"附图源件不存在：{', '.join(missing)}")
    out_dir.mkdir(parents=True, exist_ok=True)
    written = []
    for n, src in enumerate(sources, start=1):
        target = out_dir / f"fig{n}.png"
        _rasterize(src, target, chrome, scale)
        written.append(str(target))
    return written


def render_svgs(svg_dir: Path, out_dir: Path, chrome: str, scale: int) -> list[str]:
    """按 figN.svg 的编号逐图出件。"""
    sources = sorted(svg_dir.glob("fig*.svg"), key=_figure_sort_key)
    if not sources:
        raise SystemExit(f"{svg_dir} 下没有 fig*.svg")
    out_dir.mkdir(parents=True, exist_ok=True)
    written = []
    for src in sources:
        target = out_dir / f"fig{_figure_sort_key(src)}.png"
        _rasterize(src, target, chrome, scale)
        written.append(str(target))
    return written


def render_html(html: Path, out_dir: Path, chrome: str, dpi: int,
                start_page: int) -> list[str]:
    """回退路径：HTML → PDF → 排除页眉页脚后逐图裁切。"""
    try:
        import fitz  # pyright: ignore[reportMissingImports]
    except ImportError:
        raise SystemExit(
            "HTML 回退路径需要 pymupdf；请用带 pymupdf 的解释器运行，"
            "或改用 --svg-file/--svg-dir 走首选路径"
        )
    out_dir.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        pdf = Path(tmp) / "figures.pdf"
        subprocess.run(
            [chrome, "--headless", "--disable-gpu", "--no-pdf-header-footer",
             f"--print-to-pdf={pdf}", html.resolve().as_uri()],
            check=True, capture_output=True,
        )
        doc = fitz.open(str(pdf))
        written = []
        for i in range(start_page - 1, doc.page_count):
            page = doc[i]
            boxes = []
            for drawing in page.get_drawings():
                r = drawing["rect"]
                if (r.y0 > HEADER_BELOW_PT and r.y1 < FOOTER_ABOVE_PT
                        and (r.width > 2 or r.height > 2)):
                    boxes.append(r)
            for block in page.get_text("dict")["blocks"]:
                for line in block.get("lines", []):
                    for span in line["spans"]:
                        y0, y1 = span["bbox"][1], span["bbox"][3]
                        if not (HEADER_BELOW_PT < y0 and y1 < FOOTER_ABOVE_PT):
                            continue
                        if FIGURE_CAPTION.match(span["text"].strip()):
                            continue
                        boxes.append(fitz.Rect(span["bbox"]))
            if not boxes:
                continue
            union = fitz.Rect(boxes[0])
            for r in boxes[1:]:
                union |= r
            pad = 8
            union = fitz.Rect(union.x0 - pad, union.y0 - pad,
                              union.x1 + pad, union.y1 + pad) & page.rect
            target = out_dir / f"fig{i - start_page + 2}.png"
            page.get_pixmap(clip=union, dpi=dpi).save(str(target))
            written.append(str(target))
    return written


def main(argv=None):
    ap = argparse.ArgumentParser(description="附图源件 → 逐图 PNG")
    src = ap.add_mutually_exclusive_group(required=True)
    src.add_argument("--svg-file", type=Path, action="append", dest="svg_files",
                     help="首选：按给定顺序出件的 SVG 源件（可重复）")
    src.add_argument("--svg-dir", type=Path, help="首选：附图 SVG 源件目录（fig*.svg）")
    src.add_argument("--html", type=Path, help="回退：<附图>_vN.html")
    ap.add_argument("--out-dir", required=True, type=Path)
    ap.add_argument("--scale", type=int, default=3, help="SVG 路径的栅格化倍率（默认 3×）")
    ap.add_argument("--dpi", type=int, default=300, help="HTML 回退路径的 DPI")
    ap.add_argument("--start-page", type=int, default=2, help="HTML 回退路径：第 1 页为封面")
    ap.add_argument("--chrome")
    args = ap.parse_args(argv)

    chrome = find_chrome(args.chrome)
    if args.svg_files:
        written = render_files(args.svg_files, args.out_dir, chrome, args.scale)
        path = "SVG 直出（按给定顺序）"
    elif args.svg_dir:
        written = render_svgs(args.svg_dir, args.out_dir, chrome, args.scale)
        path = "SVG 直出（按 figN 编号）"
    else:
        written = render_html(args.html, args.out_dir, chrome, args.dpi, args.start_page)
        path = "HTML→PDF→裁切（回退）"
    print(json.dumps({"status": "ok", "mode": path, "figures": written},
                     ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
