#!/usr/bin/env python3
"""申请文件体例引擎：把结构化内容按 .docx 模板的体例成文。

体例真相源是**模板文件本身**（分节 / 页眉 / 页脚 PAGE 域 / 页面设置 / 段落与文字体例），
由 :mod:`style` 从模板反解。本模块只做三件事：

1. 校验 spec 与 content 的接缝（节类型、来源键、附图下标、编号口径）；
2. 按 spec 的节序，清空模板各节里的占位段落（分节符段落一律保留）；
3. 按节插入内容（正文段 / 标题 / 表格 / 图片）；
4. 套用模板反解出的体例。

用法::

    python3 build.py --spec spec/申请文件.json --content content.json \\
        --template 申请文件模板.docx --out 申请文件.docx

``content.json`` 的结构（由 ``build_patent_filing`` 工具产出，字段含义同该工具的内容模型）::

    {
      "abstract":      ["段落", ...],
      "claims":        ["1. …", ...],
      "specification": [{"kind": "h3"|"h4"|"p"|"table", "text": ..., "rows": ...}, ...],
      "figures":       ["/abs/fig1.png", ...],
      "abstractFigureIndex": 0
    }

``spec`` 可选键 ``specification_title`` 声明说明书名称的取法（``from`` 取 content 的哪个序列、
``pattern`` 的第 1 个捕获组即名称、``max_chars`` 上限、``section`` 落在哪一节）：给出时在该节
最前面写名称行（左右居中、显式零缩进）并在其后空一行，名称行不占段落编号
（《专利审查指南》第一部分第一章 4.2）。

``abstractFigureIndex`` 是可选字段：声明 ``figure_index_from`` 的单图节（本 spec 的「摘要附图」）
按它取图，缺省回落到 spec 的 ``figure_index``。

标准输出是一份 JSON 报告（``status``/``sections``/``numbering_total``/
``upstream_numbering_seen``/``template_style``）；失败时以非零退出码结束，错误写 stderr。
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path

from docx import Document
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Cm
from docx.text.paragraph import Paragraph

from style import resolve_style, template_style

WP = qn("w:p")

# 表题段形态：由 contentFromDraft 按「表 N · 名称」生成。表题不进入 [NNNN] 段落编号序列
# （不编号也不占号），与 HTML 通道的 <caption> 一致。
CAPTION_PATTERN = re.compile(r"^表 \d+ · ")


# --------------------------------------------------------------------------- 体例原语

def _pPr(style: dict, align: str | None = None, indent: bool = True):
    """段落属性：行距 + 首行缩进 + 对齐，全部取自模板反解的体例。

    子元素顺序遵循 ECMA-376 的 ``CT_PPr`` 序列（spacing → ind → jc）。此前把 ``w:jc``
    写在最前，schema 层不合规；Word 的容错度本机无从实测，不该由成品来赌。

    ``indent=False`` 写**显式零缩进**：模板 Normal 样式自带
    ``<w:ind w:firstLine="560" w:firstLineChars="200"/>``，整段不写 ``w:ind`` 的段落会
    继承该值而被静默推右，故顶格段（如说明书名称行）必须显式写 0。
    """
    pPr = OxmlElement("w:pPr")
    sp = OxmlElement("w:spacing")
    sp.set(qn("w:line"), str(int(round(style["line_spacing"] * 240))))
    sp.set(qn("w:lineRule"), "auto")
    pPr.append(sp)
    ind = OxmlElement("w:ind")
    ind.set(qn("w:firstLine"),
            str(int(round(style["first_line_indent"] * 20))) if indent else "0")
    ind.set(qn("w:firstLineChars"), "0")
    pPr.append(ind)
    if align:
        jc = OxmlElement("w:jc")
        jc.set(qn("w:val"), align)
        pPr.append(jc)
    return pPr


def _rPr(style: dict, bold: bool = False, size_pt: float | None = None):
    """文字属性：中西文字体 + 字号，全部取自模板反解的体例。"""
    rPr = OxmlElement("w:rPr")
    fonts = OxmlElement("w:rFonts")
    for attr, key in (("w:ascii", "ascii"), ("w:hAnsi", "ascii"),
                      ("w:eastAsia", "eastAsia"), ("w:cs", "cs")):
        if key in style:
            fonts.set(qn(attr), style[key])
    rPr.append(fonts)
    if bold:
        rPr.append(OxmlElement("w:b"))
    half = str(int(round((size_pt if size_pt is not None else style["size_pt"]) * 2)))
    for tag in ("w:sz", "w:szCs"):
        el = OxmlElement(tag)
        el.set(qn("w:val"), half)
        rPr.append(el)
    return rPr


def paragraph(style: dict, text: str = "", *, bold=False, align=None, size_pt=None,
              indent=True):
    p = OxmlElement("w:p")
    p.append(_pPr(style, align, indent))
    if text:
        run = OxmlElement("w:r")
        run.append(_rPr(style, bold, size_pt))
        t = OxmlElement("w:t")
        t.set(qn("xml:space"), "preserve")
        t.text = text
        run.append(t)
        p.append(run)
    return p


def table(style: dict, rows: list[list[str]], width_twips: int | None = None):
    """一个表格：网格定义 + 表宽 + 全框线 + 首行表头。

    ``w:tblGrid`` 是 Word 与 python-docx 都读的网格定义（缺它时 python-docx 取
    ``table.columns`` 直接抛 ``InvalidXmlError``），按等宽列写入；``width_twips`` 给出时
    同时写表宽（``w:tblW``）。
    """
    tbl = OxmlElement("w:tbl")
    pr = OxmlElement("w:tblPr")
    if width_twips is not None and rows:
        tbl_w = OxmlElement("w:tblW")
        tbl_w.set(qn("w:w"), str(int(width_twips)))
        tbl_w.set(qn("w:type"), "dxa")
        pr.append(tbl_w)
    borders = OxmlElement("w:tblBorders")
    for side in ("top", "left", "bottom", "right", "insideH", "insideV"):
        e = OxmlElement(f"w:{side}")
        e.set(qn("w:val"), "single")
        e.set(qn("w:sz"), "4")
        e.set(qn("w:color"), "000000")
        borders.append(e)
    pr.append(borders)
    tbl.append(pr)
    if rows:
        columns = len(rows[0])
        grid = OxmlElement("w:tblGrid")
        if width_twips is not None and columns:
            base, rest = divmod(int(width_twips), columns)
            widths = [base] * columns
            widths[-1] += rest
        else:
            widths = [0] * columns
        for width in widths:
            col = OxmlElement("w:gridCol")
            col.set(qn("w:w"), str(width))
            grid.append(col)
        tbl.append(grid)
    for r_i, row in enumerate(rows):
        tr = OxmlElement("w:tr")
        for cell in row:
            tc = OxmlElement("w:tc")
            tcp = OxmlElement("w:tcPr")
            jc = OxmlElement("w:jc")
            jc.set(qn("w:val"), "center")
            tcp.append(jc)
            tc.append(tcp)
            tc.append(paragraph(style, cell, bold=(r_i == 0), align="center",
                                size_pt=style.get("table_size_pt", style["size_pt"])))
            tr.append(tc)
        tbl.append(tr)
    return tbl


def page_break(style: dict):
    p = OxmlElement("w:p")
    p.append(_pPr(style))
    run = OxmlElement("w:r")
    br = OxmlElement("w:br")
    br.set(qn("w:type"), "page")
    run.append(br)
    p.append(run)
    return p


# --------------------------------------------------------------------------- 分节机制

class Template:
    """模板的各节边界。

    一个段落若其 pPr 内含 sectPr，它就是**该节的最后一段**；文档末节由 body 级
    sectPr 结束。插入内容时必须锚定到正确的分节符，否则内容会落进前一节——
    这是本项目已记录的真实缺陷（图 1–图 5 曾落进「说明书」节）。
    """

    def __init__(self, path: Path):
        self.doc = Document(str(path))
        self.body = self.doc.element.body
        paras = [el for el in self.body if el.tag == WP]
        self.enders = [
            p for p in paras
            if p.find(qn("w:pPr")) is not None
            and p.find(qn("w:pPr")).find(qn("w:sectPr")) is not None
        ]
        bounds, prev = [], 0
        for ender in self.enders:
            idx = paras.index(ender)
            bounds.append((prev, idx))
            prev = idx + 1
        bounds.append((prev, len(paras)))
        self.paras = paras
        self.bounds = bounds
        self.body_sect = self.body.find(qn("w:sectPr"))

    def anchor(self, index: int):
        """第 index 节的插入锚点：该节分节符段落；末节返回 None（插到 body 级 sectPr 前）。"""
        return self.enders[index] if index < len(self.enders) else None

    def clear(self, index: int):
        """清空第 index 节的占位段落，保留其分节符段落。"""
        start, stop = self.bounds[index]
        ender = self.anchor(index)
        for p in self.paras[start:stop]:
            if p is ender:
                continue
            p.getparent().remove(p)
        return ender

    def insert(self, anchor, element):
        if anchor is not None:
            anchor.addprevious(element)
        elif self.body_sect is not None:
            self.body_sect.addprevious(element)
        else:
            self.body.append(element)

    def add_picture(self, anchor, image: Path, width_cm: float, style: dict):
        holder = OxmlElement("w:p")
        # 图片段写**显式零缩进**：居中内容若带首行缩进，渲染器会把整幅图右移一个缩进量。
        # t63 受控 A/B（同一份 docx，只把含 w:drawing 的段落 firstLine 480→0）：LibreOffice
        # 实测图位 x 由 123.05 pt 变为 99.05 pt，差值恰为 24.00 pt（＝480 twips）。
        holder.append(_pPr(style, "center", indent=False))
        self.insert(anchor, holder)
        Paragraph(holder, self.doc).add_run().add_picture(str(image), width=Cm(width_cm))


# --------------------------------------------------------------------------- 主流程

# spec 的节类型与正文节点类别的封闭集合；未定义取值一律报错，不回落成别的类型。
SECTION_KINDS = ("text", "mixed", "figure")
NODE_KINDS = ("h3", "h4", "p", "table")
NUMBERING_POLICIES = ("none", "paragraph")


def single_figure_index(sec: dict, content: dict):
    """单图节的图序：spec 声明 ``figure_index_from`` 时从 content 取（「摘要附图」由模型指定
    第几张），content 未给该键时回落到 spec 的 ``figure_index``。

    Args:
        sec: spec 里的一个 ``kind`` 为 ``figure`` 且不带 ``figure_all`` 的节。
        content: 结构化内容模型。

    Returns:
        ``(图序, 来源名)``；来源名用于报错文案。
    """
    source = sec.get("figure_index_from")
    if source in content:
        return content[source], f"content.{source}"
    return sec.get("figure_index"), "figure_index"


def validate_spec(spec: dict, content: dict) -> re.Pattern:
    """校验 spec 与 content 的接缝，返回段落编号的匹配式。

    spec 是随包资产、content 由模型产出，两者都在本模块的输入边界上。缺键与未定义取值
    必须在这里报错，不能回落：``sec.get("kind", "text")`` 那样写会把一个拼错的节类型变成
    一个被清空、什么都没写的节，而成文与验收都报成功——空摘要就是这样交付出去的。

    Args:
        spec: 文书 spec。
        content: 结构化内容模型。

    Returns:
        由 ``spec.assertions.numbering_pattern`` 编译的段落编号匹配式。
    """
    for key in ("sections", "style", "figure_width_cm", "heading_bold", "assertions"):
        if key not in spec:
            raise SystemExit(f"spec 缺 {key}")
    pattern = spec["assertions"]["numbering_pattern"]
    numbering = re.compile(pattern)
    if numbering.groups != 1:
        raise SystemExit(f"spec 的 numbering_pattern {pattern!r} 必须恰好含 1 个捕获组")

    for i, sec in enumerate(spec["sections"], 1):
        for key in ("key", "from", "kind"):
            if key not in sec:
                raise SystemExit(f"spec 第 {i} 节缺 {key}")
        where = f"第 {i} 节（key={sec['key']}）"
        if sec["kind"] not in SECTION_KINDS:
            raise SystemExit(f"spec {where} 的 kind {sec['kind']!r} 未定义；可用：{'、'.join(SECTION_KINDS)}")
        if sec["from"] not in content:
            raise SystemExit(f"spec {where} 的 from {sec['from']!r} 不在 content 里")
        if sec["kind"] in ("text", "mixed"):
            if sec.get("numbering") not in NUMBERING_POLICIES:
                raise SystemExit(
                    f"spec {where} 的 numbering {sec.get('numbering')!r} 未定义；可用：{'、'.join(NUMBERING_POLICIES)}"
                )
        if sec["kind"] == "figure" and not sec.get("figure_all"):
            index, origin = single_figure_index(sec, content)
            if not isinstance(index, int) or isinstance(index, bool) or not 0 <= index < len(content["figures"]):
                raise SystemExit(
                    f"spec {where} 的 {origin} {index!r} 超出 content.figures（{len(content['figures'])} 张）"
                )
        if sec["kind"] == "mixed":
            for node in content[sec["from"]]:
                if node.get("kind") not in NODE_KINDS:
                    raise SystemExit(
                        f"spec {where} 的 content 节点 kind {node.get('kind')!r} 未定义；可用：{'、'.join(NODE_KINDS)}"
                    )
    title = spec.get("specification_title")
    if title is not None:
        if not isinstance(title, dict):
            raise SystemExit("spec 的 specification_title 不是对象")
        for key in ("from", "pattern"):
            if key not in title:
                raise SystemExit(f"spec 的 specification_title 缺 {key}")
        if title["from"] not in content:
            raise SystemExit(
                f"spec 的 specification_title.from {title['from']!r} 不在 content 里"
            )
        try:
            title_pattern = re.compile(title["pattern"])
        except re.error as exc:
            raise SystemExit(f"spec 的 specification_title.pattern 不是合法正则：{exc}")
        if title_pattern.groups != 1:
            raise SystemExit("spec 的 specification_title.pattern 必须恰好含 1 个捕获组")
        index = next(
            (i for i, sec in enumerate(spec["sections"])
             if sec["key"] == title.get("section", "specification")),
            None,
        )
        if index is None:
            raise SystemExit(
                f"spec 的 specification_title.section {title.get('section', 'specification')!r}"
                " 不在 spec 的节里"
            )
    return numbering


def specification_title(spec: dict, content: dict) -> str | None:
    """说明书名称（spec 声明 ``specification_title`` 时），否则 ``None``。

    名称由 ``pattern`` 的第 1 个捕获组决定，取自 ``from`` 指明的正文序列的第 1 项——本 spec
    取权利要求第 1 项，使**说明书名称与权利要求 1 的主题名称必然一致**（《专利法实施细则》
    第二十条第一款要求名称与请求书一致，这里是它的可执行化）。取不到、为空或超过
    ``max_chars`` 一律报错：名称是法定记载事项，不能静默回落成"没有名称"。

    Args:
        spec: 文书 spec。
        content: 结构化内容模型。

    Returns:
        名称文本；spec 未声明该键时 ``None``。
    """
    rule = spec.get("specification_title")
    if rule is None:
        return None
    source = content[rule["from"]]
    first = source[0] if source else ""
    found = re.compile(rule["pattern"]).search(first)
    if found is None:
        raise SystemExit(
            f"spec 的 specification_title.pattern {rule['pattern']!r} 在 "
            f"content.{rule['from']}[0] {first[:40]!r} 上不匹配，说明书名称无法确定"
        )
    title = found.group(1).strip()
    limit = int(rule.get("max_chars", 25))
    if not title or len(title) > limit:
        raise SystemExit(
            f"说明书名称 {title!r} 为空或超过 spec 声明的 max_chars={limit}，"
            "请核对 spec.specification_title"
        )
    return title


def table_width_twips(doc) -> int:
    """文档首节的版心宽度（twips）：页宽减左右页边距；取不到时 0。

    1 twip = 635 EMU；模板实测 11906 − 1800 × 2 = 8306。
    """
    try:
        section = doc.sections[0]
        usable = section.page_width - section.left_margin - section.right_margin
    except (IndexError, TypeError, ValueError):
        return 0
    return int(usable // 635) if usable else 0


def build(spec: dict, content: dict, template_path: Path, out_path: Path) -> dict:
    """按模板体例成文。

    Args:
        spec: 文书 spec（节序、编号口径、附图宽度、结构性断言）。
        content: 结构化内容模型。
        template_path: 模板 .docx 路径（体例真相源）。
        out_path: 产物 .docx 路径。

    Returns:
        报告：各节承载的段落/图片数、段落编号总数、从源件读到的上游编号数、模板体例。
    """
    style = resolve_style(spec, template_path)
    numbering_pattern = validate_spec(spec, content)
    tpl = Template(template_path)
    if len(tpl.bounds) != len(spec["sections"]):
        raise SystemExit(
            f"模板分节数 {len(tpl.bounds)} 与 spec 声明的节数 {len(spec['sections'])} 不一致"
        )

    report = {"sections": [], "template_style": template_style(template_path)}
    numbering = 0
    upstream_seen = 0
    upstream_mismatch: list[str] = []
    heading_bold = spec["heading_bold"]
    title = specification_title(spec, content)
    title_section = (spec.get("specification_title") or {}).get("section", "specification")
    table_width = table_width_twips(tpl.doc)

    for i, sec in enumerate(spec["sections"]):
        anchor = tpl.clear(i)
        kind = sec["kind"]
        added = {"key": sec["key"], "paragraphs": 0, "figures": 0}

        if kind == "text":
            for text in content[sec["from"]]:
                tpl.insert(anchor, paragraph(style, text))
                added["paragraphs"] += 1

        elif kind == "mixed":
            if title is not None and sec["key"] == title_section:
                # 《审查指南》第一部分第一章 4.2：说明书第一页第一行写明发明名称（左右居中、
                # 前面不冠"名称"字样），名称与正文之间空一行；名称行不占段落编号。
                tpl.insert(anchor, paragraph(style, title, align="center", indent=False))
                tpl.insert(anchor, paragraph(style, ""))
                added["paragraphs"] += 1
            for node in content[sec["from"]]:
                node_kind = node["kind"]
                if node_kind == "table":
                    tpl.insert(anchor, table(style, node["rows"], table_width))
                elif node_kind in ("h3", "h4"):
                    tpl.insert(anchor, paragraph(style, node["text"], bold=heading_bold))
                else:
                    text = node["text"]
                    if sec.get("numbering") == "paragraph" and not CAPTION_PATTERN.match(text):
                        numbering += 1
                        expected = f"[{numbering:04d}]"
                        # 上游渲染引擎也会写编号。先剥后写，既幂等，也让本引擎保持
                        # 对 docx 的最终决定权；若上游编号与本引擎的序号不一致，
                        # 说明 HTML 与 docx 会分叉，直接报错而不是静默覆盖。
                        found = numbering_pattern.match(text)
                        if found:
                            upstream_seen += 1
                            if found.group(0).strip() != expected:
                                upstream_mismatch.append(
                                    f"第 {numbering} 段：上游为 {found.group(0).strip()}，本引擎为 {expected}"
                                )
                            text = text[found.end():].lstrip()
                        text = f"{expected} {text}"
                    tpl.insert(anchor, paragraph(style, text))
                added["paragraphs"] += 1

        elif kind == "figure":
            figures = content["figures"]
            if sec.get("figure_all"):
                selected = figures
            else:
                selected = [figures[single_figure_index(sec, content)[0]]]
            for n, image in enumerate(selected):
                if n > 0 and sec.get("one_per_page"):
                    tpl.insert(None if anchor is None else anchor, page_break(style))
                tpl.add_picture(anchor, Path(image), spec["figure_width_cm"], style)
                added["figures"] += 1

        else:
            raise SystemExit(f"引擎缺节类型 {kind!r} 的处理分支（spec 第 {i + 1} 节）")

        report["sections"].append(added)

    if upstream_mismatch:
        raise SystemExit(
            "上游 HTML 的段落编号与本引擎序号不一致，HTML 与 docx 会分叉：\n  "
            + "\n  ".join(upstream_mismatch[:10])
        )
    report["numbering_total"] = numbering
    report["upstream_numbering_seen"] = upstream_seen
    write_atomically(tpl.doc, out_path)
    return report


def write_atomically(doc, out_path: Path) -> None:
    """把成品写到同目录的临时件后改名，避免交付路径上留下半写的 .docx。

    直接 ``save(out_path)`` 会在保存途中覆盖已存在的成品：失败时那里留着的是一个截断的
    zip，而文件名与成功产物无从区分。

    Args:
        doc: 待保存的 ``docx.Document``。
        out_path: 成品目标路径。
    """
    target = Path(out_path)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_name(f".{target.name}.part")
    try:
        doc.save(str(tmp))
        os.replace(tmp, target)
    except BaseException:
        tmp.unlink(missing_ok=True)
        raise


def main(argv=None):
    ap = argparse.ArgumentParser(description="按模板体例把结构化内容成文为 .docx")
    ap.add_argument("--spec", required=True, type=Path)
    ap.add_argument("--content", required=True, type=Path)
    ap.add_argument("--template", required=True, type=Path)
    ap.add_argument("--out", required=True, type=Path)
    args = ap.parse_args(argv)

    spec = json.loads(args.spec.read_text(encoding="utf-8"))
    content = json.loads(args.content.read_text(encoding="utf-8"))
    report = build(spec, content, args.template, args.out)
    print(json.dumps({"status": "ok", "out": str(args.out), **report},
                     ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
