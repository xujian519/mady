#!/usr/bin/env python3
"""申请文件成品验收：对生成/交付的 .docx 跑体例断言与内容断言。

退出码：0 全部通过；1 有 error。

用法::

    python3 verify.py --spec spec/申请文件.json --docx 申请文件.docx
    python3 verify.py --spec spec/申请文件.json --docx 申请文件.docx \\
        --template 申请文件模板.docx     # 额外做"模板体例未漂移"核对

断言分三类：

* **体例**：分节数 / 各节页眉 / 行距 / 首行缩进 / 字号 / 无非黑色文字。
  这些是"静默缺陷"高发区——模型看不出来，源件的 `docs check` 也不覆盖。
* **内容**：说明书五部分齐备 / 说明书名称（spec 声明 ``specification_title`` 时：第一行
  ＝权利要求 1 的主题名称、左右居中、其后空一行）/ 权利要求项数 / ``[NNNN]`` 编号连续
  （**按 spec 的 numbering 政策生效**：没有节声明编号时该断言无主体）/ 附图数量 /
  每一节都承载了内容（空节是静默缺陷：成文时某一节的类型或来源键错了，成品会少一整节，
  而分节数、页眉、编号连续性全都照样通过）。
* **痕迹**：内部复核标记、待补占位、QA 注记一律不得出现。

标准输出是一份 JSON（``passed`` / ``errors`` / ``info``）。
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

from docx import Document
from docx.oxml.ns import qn
from docx.text.paragraph import Paragraph

from style import template_style

WP = qn("w:p")


def _iter_body(doc):
    """按 body 顺序产出 ('p', element) / ('tbl', element)。"""
    for el in doc.element.body:
        if el.tag == WP:
            yield "p", el
        elif el.tag == qn("w:tbl"):
            yield "tbl", el


def _iter_paragraph_elements(doc):
    """按 body 顺序产出全部段落元素，包括表格单元格里的段落。

    ``python-docx`` 的 ``doc.paragraphs`` 不含表格单元格，只遍历它会让表格里的字号、
    颜色、内部复核标记与权利要求项逃过验收。
    """
    for kind, el in _iter_body(doc):
        if kind == "p":
            yield el
        else:
            yield from el.iter(WP)


def _text(el) -> str:
    return "".join(t.text or "" for t in el.iter(qn("w:t")))


def section_layout(doc):
    """各节的 (段落数, 图片数)，用于判断内容是否落进了正确的节。"""
    layout, current = [], 0
    layout.append([0, 0])
    for kind, el in _iter_body(doc):
        if kind == "p":
            pPr = el.find(qn("w:pPr"))
            if pPr is not None and pPr.find(qn("w:sectPr")) is not None:
                current += 1
                layout.append([0, 0])
                continue
            if el.findall(".//" + qn("w:drawing")):
                layout[current][1] += 1
            elif _text(el).strip():
                layout[current][0] += 1
        else:
            layout[current][0] += 1
    return layout


def section_paragraphs(doc):
    """按 body 顺序产出各节的段落元素列表（分节符段落归入它所结束的那一节）。

    ``section_layout`` 只给每节的 (段数, 图数)；核对"说明书第一行是不是名称"需要拿到具体
    元素，故另出本函数。
    """
    sections: list[list] = [[]]
    for kind, el in _iter_body(doc):
        if kind != "p":
            continue
        pPr = el.find(qn("w:pPr"))
        sections[-1].append(el)
        if pPr is not None and pPr.find(qn("w:sectPr")) is not None:
            sections.append([])
    return sections


def verify(spec: dict, docx_path: Path, template_path: Path | None = None) -> dict:
    """对成品 .docx 跑断言。

    Args:
        spec: 文书 spec，读取其 ``assertions`` 与 ``style``。
        docx_path: 待验收的 .docx 路径。
        template_path: 提供时额外核对成品体例与模板反解体例是否一致（模板漂移防线）。

    Returns:
        ``errors`` 为断言失败列表（空即通过），``info`` 为实测摘要。
    """
    errors: list[str] = []
    d = Document(str(docx_path))
    a = spec["assertions"]
    style = spec["style"]
    body_text = "\n".join(_text(el) for el in _iter_paragraph_elements(d))

    # spec 声明 specification_title 时，说明书第一行是"顶格"的名称行：按段核对缩进时要豁免它
    # （它显式写 0），并对它单独做三项断言（文本＝权1 主题名称／左右居中／其后空一行）。
    title_rule = spec.get("specification_title")
    title_sections: list[list] = []
    title_index = None
    title_element = None
    if title_rule is not None:
        title_sections = section_paragraphs(d)
        title_index = next(
            (i for i, sec in enumerate(spec["sections"])
             if sec["key"] == title_rule.get("section", "specification")),
            None,
        )
        if title_index is not None and title_index < len(title_sections):
            filled = [el for el in title_sections[title_index] if _text(el).strip()]
            title_element = filled[0] if filled else None

    # 顶格豁免**锚定在"居中"这一要求上**：只有"说明书节第一段 + 左右居中"才是《审查指南》
    # 一部一章 4.2 的名称行。否则（例如旧件的第一段恰是"技术领域"标题）不该把那条段落的缩进
    # 期望值改成 0 —— 那会给一份体例正常的旧件多报一条假的缩进错。
    title_exempt = False
    if title_element is not None:
        _pPr = title_element.find(qn("w:pPr"))
        _jc = _pPr.find(qn("w:jc")) if _pPr is not None else None
        title_exempt = _jc is not None and _jc.get(qn("w:val")) == "center"

    # ---- 体例：分节与页眉
    headers = []
    for s in d.sections:
        paras = s.header.paragraphs
        headers.append(paras[0].text.strip() if paras else "")
    if len(d.sections) != a["section_count"]:
        errors.append(f"分节数 {len(d.sections)} ≠ 期望 {a['section_count']}")
    if headers != a["headers"]:
        errors.append(f"各节页眉不匹配：{headers} ≠ {a['headers']}")

    # ---- 体例：段落格式（含表格单元格）
    n_par = 0
    for el in _iter_paragraph_elements(d):
        p = Paragraph(el, d)
        if not p.text.strip():
            continue
        n_par += 1
        pf = p.paragraph_format
        if pf.line_spacing is not None and abs(pf.line_spacing - style["line_spacing"]) > 1e-6:
            errors.append(f"行距 {pf.line_spacing} ≠ {style['line_spacing']}：{p.text[:18]!r}")
        # 图片段与名称行都要求**显式零缩进**：居中内容带首行缩进会被渲染器整体右移一个
        # 缩进量（t63 受控 A/B 实测 24.00 pt）。期望值随段落性质变化，断言不放宽。
        has_drawing = bool(el.findall(".//" + qn("w:drawing")))
        expected_indent = (
            0.0 if has_drawing or (title_exempt and el is title_element)
            else style["first_line_indent"]
        )
        if pf.first_line_indent is not None and abs(pf.first_line_indent.pt - expected_indent) > 1e-6:
            errors.append(f"首行缩进 {pf.first_line_indent.pt}pt ≠ {expected_indent}：{p.text[:18]!r}")
        for r in p.runs:
            if r.font.size and round(r.font.size.pt, 2) not in a["max_sizes_pt"]:
                errors.append(f"字号 {r.font.size.pt}pt 不在 {a['max_sizes_pt']}：{p.text[:18]!r}")
            if a.get("forbid_colors") and r.font.color and r.font.color.rgb:
                if str(r.font.color.rgb) != "000000":
                    errors.append(f"非黑色文字 {r.font.color.rgb}（Word 内置标题体例？）：{p.text[:18]!r}")

    # ---- 内容：说明书五部分 / 权项 / 编号
    for h in a["spec_headings"]:
        if not re.search(rf"^\s*{re.escape(h)}\s*$", body_text, re.M):
            errors.append(f"说明书缺法定部分标题：{h}")

    claims = re.findall(a["claim_pattern"], body_text, re.M)
    if not claims:
        errors.append("未识别到任何权利要求项")

    numbers = [int(x) for x in re.findall(a["numbering_pattern"], body_text, re.M)]
    # 连续性断言只在 **spec 确实声明了段落编号** 时有主体：spec 的 numbering 政策与
    # assertions.continuity 是同一体例决定的两处声明，后者按前者生效 —— 既避免"声明不编号却
    # 仍要求编号"的自相矛盾，也避免这条断言被静默关掉（声明编号时行为与改动前逐字相同）。
    expects_numbering = any(
        sec.get("numbering") == "paragraph"
        for sec in spec["sections"] if sec.get("kind") in ("text", "mixed")
    )
    if a.get("continuity") and expects_numbering:
        if not numbers:
            errors.append("说明书中没有任何 [NNNN] 段落编号")
        elif numbers != list(range(1, len(numbers) + 1)):
            errors.append(f"段落编号不连续（共 {len(numbers)} 条）")

    # ---- 内容：说明书名称（spec 声明 specification_title 时）
    if title_rule is not None:
        seed = next(
            (line for line in body_text.splitlines() if re.match(r"^\s*1[.、]\s*", line)),
            None,
        )
        expected = None
        if seed is None:
            errors.append(
                "spec 声明了 specification_title，但成品里找不到权利要求 1 的正文，无法核对说明书名称"
            )
        else:
            found = re.search(title_rule["pattern"], seed)
            if found is None:
                errors.append(
                    f"说明书名称规则 {title_rule['pattern']!r} 在权利要求 1 {seed[:24]!r} 上不匹配"
                )
            else:
                expected = found.group(1).strip()
        if title_element is None:
            errors.append("成品里找不到说明书名称行（spec 声明的节没有正文段）")
        else:
            text = _text(title_element).strip()
            if expected is not None and text != expected:
                errors.append(f"说明书第一行 {text!r} ≠ 权利要求 1 的主题名称 {expected!r}")
            pPr = title_element.find(qn("w:pPr"))
            jc = pPr.find(qn("w:jc")) if pPr is not None else None
            jc_val = jc.get(qn("w:val")) if jc is not None else None
            if jc_val != "center":
                errors.append(f"说明书名称未左右居中（w:jc={jc_val}）：{text[:18]!r}")
            ind = pPr.find(qn("w:ind")) if pPr is not None else None
            ind_val = ind.get(qn("w:firstLine")) if ind is not None else None
            if ind_val != "0":
                errors.append(
                    f"说明书名称段未显式写零首行缩进（模板 Normal 自带 560 twips）：{text[:18]!r}"
                )
            assert title_index is not None
            block = title_sections[title_index]
            pos = block.index(title_element)
            following = block[pos + 1] if pos + 1 < len(block) else None
            if following is None or _text(following).strip():
                errors.append(f"说明书名称与正文之间未空一行：{text[:18]!r}")

    # ---- 痕迹
    for bad in a["forbid_text"]:
        if bad in body_text:
            errors.append(f"内部痕迹未清除：{bad!r}")

    # ---- 结构归属
    layout = section_layout(d)
    for i, sec in enumerate(spec["sections"]):
        if i >= len(layout):
            break
        paragraphs, figures = layout[i]
        if paragraphs == 0 and figures == 0:
            errors.append(f"节 {sec['key']} 未承载任何内容（段落 0，图片 0）")
    info = {
        "sections": len(d.sections),
        "headers": headers,
        "specification_title": None if title_element is None else _text(title_element).strip(),
        "numbering_expected": expects_numbering,
        "paragraphs": n_par,
        "claims": len(claims),
        "numbering": f"1..{len(numbers)}" if numbers else "无",
        "tables": len(d.tables),
        "figures": len(d.inline_shapes),
        "layout": {spec["sections"][i]["key"]: {"paragraphs": v[0], "figures": v[1]}
                   for i, v in enumerate(layout) if i < len(spec["sections"])},
        "template": None,
    }

    # ---- 模板体例未漂移（可选）
    if template_path is not None:
        derived = template_style(template_path)
        info["template"] = {"sections": derived["section_count"],
                            "sizes_pt": derived["sizes_pt"],
                            "line_spacings": derived["line_spacings"]}
        if derived["section_count"] != a["section_count"]:
            errors.append(f"模板分节数 {derived['section_count']} ≠ spec 声明 {a['section_count']}")
        if style["size_pt"] != derived["size_pt"]:
            errors.append(f"spec 字号 {style['size_pt']}pt ≠ 模板字号 {derived['size_pt']}pt")
        if style["line_spacing"] != derived["line_spacing"]:
            errors.append(f"spec 行距 {style['line_spacing']} ≠ 模板行距 {derived['line_spacing']}")

    return {"passed": not errors, "errors": errors, "info": info}


def main(argv=None):
    ap = argparse.ArgumentParser(description="申请文件成品验收")
    ap.add_argument("--spec", required=True, type=Path)
    ap.add_argument("--docx", required=True, type=Path)
    ap.add_argument("--template", type=Path)
    args = ap.parse_args(argv)

    spec = json.loads(args.spec.read_text(encoding="utf-8"))
    result = verify(spec, args.docx, args.template)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
