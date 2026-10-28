#!/usr/bin/env python3
"""体例反解：从模板 .docx 读出字体、字号、行距、首行缩进与分节数。

模板是体例的唯一真相源。本模块的取值全部来自模板自身的 OOXML：

* 字体——``w:styles.xml`` 的 ``docDefaults/rPrDefault/rPr/rFonts``；
* 字号、行距、首行缩进——``word/document.xml`` 各段落标记的
  ``w:pPr/w:rPr/w:sz``、``w:pPr/w:spacing``、``w:pPr/w:ind``；
* 分节数——``w:pPr/w:sectPr`` 的个数加末节的 body 级 ``sectPr``。

模板的体例必须**唯一**：同一项观测到多个不同值时体例不可判定，本模块报错而不是任取一个。
页眉页脚部件不参与反解——它们的字号（页眉黑体、页脚页码域）不属于正文体例。
"""

from __future__ import annotations

import zipfile
from pathlib import Path
from xml.etree import ElementTree

W = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"

# spec 可以声明、但模板无从反解的体例项（模板里没有表格，表格字号无来源）。
DECLARED_ONLY_KEYS = ("table_size_pt",)


class StyleError(SystemExit):
    """模板体例无法反解，或 spec 声明与模板不符。"""


def _unique(values: set, label: str, template: Path):
    """取唯一观测值；观测到多个不同值时体例不可判定。"""
    if not values:
        raise StyleError(f"模板 {template} 的{label}无法反解：模板中没有该体例项")
    if len(values) > 1:
        raise StyleError(
            f"模板 {template} 的{label}不唯一：观测到 {sorted(values)}。"
            "体例真相源必须唯一，请先在模板中统一该项"
        )
    return next(iter(values))


def _rfonts(root) -> dict:
    """docDefaults 的字面字体名（段落级 rFonts 多为主题引用，字面名只在此处）。"""
    fonts = root.find(f".//{W}docDefaults/{W}rPrDefault/{W}rPr/{W}rFonts")
    if fonts is None:
        raise StyleError("模板缺少 docDefaults/rPrDefault/rPr/rFonts，无法反解字体")
    out = {}
    for attr, key in ((f"{W}ascii", "ascii"), (f"{W}eastAsia", "eastAsia"), (f"{W}cs", "cs")):
        value = fonts.get(attr)
        if value is None:
            raise StyleError(f"模板 docDefaults 的 rFonts 缺少 {attr}")
        out[key] = value
    return out


def _marks(document_root):
    """按 body 顺序产出每个段落标记的 (rPr, spacing, ind, 是否携带 sectPr)。"""
    body = document_root.find(f"{W}body")
    if body is None:
        raise StyleError("模板 document.xml 缺少 body")
    for para in body.findall(f"{W}p"):
        ppr = para.find(f"{W}pPr")
        rpr = ppr.find(f"{W}rPr") if ppr is not None else None
        spacing = ppr.find(f"{W}spacing") if ppr is not None else None
        ind = ppr.find(f"{W}ind") if ppr is not None else None
        sect = ppr.find(f"{W}sectPr") if ppr is not None else None
        yield rpr, spacing, ind, sect is not None


def template_style(path: Path) -> dict:
    """从模板反解体例，或指出模板体例不唯一/不完整。

    Args:
        path: 模板 .docx 路径。

    Returns:
        ``section_count`` / ``eastAsia`` / ``ascii`` / ``cs`` / ``size_pt`` /
        ``line_spacing`` / ``first_line_indent``，以及观测集合 ``sizes_pt``、
        ``line_spacings``（供调用方在报错信息中列出实际观测）。
    """
    path = Path(path)
    with zipfile.ZipFile(path) as archive:
        styles = ElementTree.fromstring(archive.read("word/styles.xml"))
        document = ElementTree.fromstring(archive.read("word/document.xml"))

    sizes: set[float] = set()
    lines: set[float] = set()
    indents: set[float] = set()
    enders = 0
    for rpr, spacing, ind, is_ender in _marks(document):
        if is_ender:
            enders += 1
        if rpr is not None:
            sz = rpr.find(f"{W}sz")
            size = sz.get(f"{W}val") if sz is not None else None
            if size is not None:
                sizes.add(int(size) / 2)
        if spacing is not None:
            line = spacing.get(f"{W}line")
            if line is not None:
                value = int(line)
                rule = spacing.get(f"{W}lineRule") or "auto"
                # 倍数行距按 OOXML 口径换算（240 twips = 1 倍）；定值行距保留 twips。
                lines.add(round(value / 240, 3) if rule == "auto" else float(value))
        if ind is not None:
            first_line = ind.get(f"{W}firstLine")
            if first_line is not None:
                indents.add(int(first_line) / 20)

    fonts = _rfonts(styles)
    return {
        "section_count": enders + 1,
        "eastAsia": fonts["eastAsia"],
        "ascii": fonts["ascii"],
        "cs": fonts["cs"],
        "size_pt": _unique(sizes, "正文字号", path),
        "line_spacing": _unique(lines, "行距", path),
        "first_line_indent": _unique(indents, "首行缩进", path),
        "sizes_pt": sorted(sizes),
        "line_spacings": sorted(lines),
    }


def resolve_style(spec: dict, template_path: Path) -> dict:
    """以模板体例为准解析出生效体例，并核对 spec 的声明。

    spec 的 ``style`` 是对模板体例的投影，不是第二个真相源：模板能反解的每一项都必须与
    spec 声明一致，不一致即报错（模板漂移在成文之前就被拦住，而不是留到验收阶段）。

    Args:
        spec: 文书 spec，读取其 ``style`` 块。
        template_path: 模板 .docx 路径。

    Returns:
        生效体例：模板反解出的字体/字号/行距/首行缩进，加上 spec 声明的模板无从反解项。
    """
    derived = template_style(template_path)
    declared = spec["style"]
    for key in ("eastAsia", "ascii", "cs", "size_pt", "line_spacing", "first_line_indent"):
        if key not in declared:
            continue
        if declared[key] != derived[key]:
            raise StyleError(
                f"spec 声明的 {key}={declared[key]!r} 与模板反解值 {derived[key]!r} 不一致"
                f"（模板 {template_path}）；体例真相源是模板，请改 spec 或改模板"
            )
    effective = {
        "eastAsia": derived["eastAsia"],
        "ascii": derived["ascii"],
        "cs": derived["cs"],
        "size_pt": derived["size_pt"],
        "line_spacing": derived["line_spacing"],
        "first_line_indent": derived["first_line_indent"],
    }
    for key in DECLARED_ONLY_KEYS:
        if key in declared:
            effective[key] = declared[key]
    return effective
