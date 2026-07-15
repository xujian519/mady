#!/usr/bin/env python3
"""
Extract invalidation-decision test cases from the 宝宸知识库_Raw dataset.

Source: /Users/xujian/projects/宝宸知识库_Raw/无效复审决定/*.md (31562 files)
Output: agentcore/evaluate/benchmark/invalidation_decisions.json

Each output TestCase contains:
  - Input: patent info + claim 1 (full) + evidence summary + invalidation grounds
  - Expected: conclusion + core reasoning + legal articles
  - RequiredCitations: the patent-law articles cited in the decision

Unlike the previous P2B (which had empty claims/evidence/reason fields due to
wrong regex anchors), this version uses the correct anchors from the real MD
files: "权利要求书如下" / "证据N：" / "无效理由为" / the final decision line.
"""
import os, re, json, random, sys

DATA_DIR = '/Users/xujian/projects/宝宸知识库_Raw/无效复审决定'
OUT_PATH = '/Users/xujian/projects/Mady/agentcore/evaluate/benchmark/invalidation_decisions.json'
TARGET_COUNT = 100  # expanded from 40; dataset has 31k+ files


def read_text(filepath):
    with open(filepath, encoding='utf-8') as f:
        return f.read()


def classify_patent_type(text):
    """Classify by patent type mentioned in the decision text."""
    if re.search(r'外观设计', text):
        return '外观设计'
    if re.search(r'实用新型', text):
        return '实用新型'
    if re.search(r'发明', text):
        return '发明'
    return None


def extract_verdict(text):
    """Extract the actual decision from the body (not the template header).

    The MD files start with a fixed 3-option template (lines 9-13):
      宣告专利权全部无效。/ 宣告专利权部分无效。/ 维持专利权有效。
    The ACTUAL decision appears later in the body, typically near the end as:
      "宣告XXXX号发明专利权全部无效" or "维持XXXX号发明专利权有效"
      or "宣告...部分无效，在...基础上继续维持该专利权有效"

    Strategy: search the LAST occurrence of a substantive decision statement
    (must contain a patent number or "权利要求" to distinguish from template).
    """
    # Find all decision-like statements in the body (skip the header template)
    # The header template is in the first ~600 chars; body decisions are later.
    body = text[600:] if len(text) > 600 else text

    # Look for the conclusive statement near the end of the decision section
    # Pattern: 宣告/维持 + patent number + 无效/有效
    patterns = [
        (r'宣告.{0,80}专利权.{0,10}全部无效', '全部无效'),
        (r'宣告.{0,80}权利要求.{0,30}全部无效', '全部无效'),
        (r'宣告.{0,80}专利权部分无效', '部分无效'),
        (r'在.{0,80}权利要求.{0,30}基础上.{0,10}继续维持', '部分无效'),
        (r'维持.{0,80}专利权有效', '维持有效'),
        (r'无效宣告请求.{0,10}理由.{0,10}均不成立', '维持有效'),
        (r'维持.{0,80}号.{0,10}发明专利权.{0,5}有效', '维持有效'),
        (r'维持.{0,80}号.{0,10}实用新型专利权.{0,5}有效', '维持有效'),
    ]

    # Search from end backwards: find the last matching pattern in body
    for pat, label in patterns:
        matches = list(re.finditer(pat, body))
        if matches:
            # Verify this isn't in the header template by checking it has substance
            last_match = matches[-1].group()
            # Must contain patent number or claim reference (not bare template)
            if re.search(r'\d{5,}|权利要求|号', last_match):
                return label

    # Fallback: check the very last 500 chars for any decision keyword
    tail = text[-500:]
    if '全部无效' in tail and '部分无效' not in tail:
        return '全部无效'
    if '部分无效' in tail or '继续维持' in tail:
        return '部分无效'
    if '维持' in tail and '有效' in tail:
        return '维持有效'
    return None


def extract_patent_number(filepath, text):
    """Extract patent number. Prefer text body (专利号为XXXX), fallback filename."""
    # Primary: search text for patent number pattern
    m = re.search(r'专利号\s*为\s*(\d{6,}[A-Z0-9.]*\d?)', text)
    if m:
        return m.group(1).strip()
    m = re.search(r'专利号\s*[:：]\s*(\d{6,}[A-Z0-9.]*\d?)', text)
    if m:
        return m.group(1).strip()
    # Fallback: from filename — extract the numeric patent number
    # Filename: {案件编号}_{专利号}.md or just {专利号}.md
    basename = os.path.basename(filepath).replace('.md', '')
    # Try: last segment after _ that looks like a patent number (digits + optional X)
    parts = basename.split('_')
    for part in reversed(parts):
        # Patent numbers: 7-13 digits, optionally ending with X or containing dots
        if re.match(r'^\d{7,13}[A-Z]?$', part) or re.match(r'^\d{6,}\.\d$', part):
            return part
    # Last resort: first pure-digit segment
    for part in parts:
        if re.match(r'^\d{7,}$', part):
            return part
    return basename


def extract_claims(text):
    """Extract claim 1 from the patent's claims section.

    Strategy: find the first numbered claim "1." or "1、" that appears after
    a claims-section marker. Real data shows varied anchors and numbering, but
    the claim itself always starts with "1" + punctuation inside a quoted block.
    """
    # Find all occurrences of claim-1 patterns: "1." or "1、" or "1．" followed by content
    # The claims section is always in the first half of the document (before the decision analysis)
    for m in re.finditer(r'(?:["""]\s*\n?\s*)?(1[\.、．]\s*(?:一种|一个|方法|装置|系统|化合物|组合物|用途|氨法|一体).{15,}?)(?=\n\s*[2２][\.、．]|"\s*\n|\n\s*请求人|\n\s*针对|\n\s*经形式)', text[:len(text)//2], re.DOTALL):
        claim = m.group(1).strip().strip('""""')
        # Verify this looks like a claim (should contain technical content)
        if len(claim) > 20:
            return _truncate(claim, 600)

    # Fallback: simpler pattern — "1." followed by content until "2." or end of claims section
    m = re.search(r'(1[\.、．]\s*.{20,}?)(?=\n\s*[2２][\.、．]|"\s*\n\s*请求人|\n\s*针对上述)', text[:len(text)//2], re.DOTALL)
    if m:
        claim = m.group(1).strip().strip('""""')
        return _truncate(claim, 600)

    return ''


def extract_evidence(text):
    """Extract evidence list (prior-art documents submitted by requester)."""
    # Pattern: 证据1：...，公开日期为...
    evidence = []
    for m in re.finditer(r'证据\s*(\d+)\s*[:：]\s*(.*?)(?=证据\s*\d+\s*[:：]|经形式审查|请求人提交意见|请求人于|\n\n)', text, re.DOTALL):
        num = m.group(1)
        desc = m.group(2).strip().replace('\n', ' ')
        if len(desc) > 3:  # skip empty
            evidence.append(f'证据{num}：{_truncate(desc, 120)}')
    if evidence:
        return '; '.join(evidence[:8])  # max 8 evidence items
    return ''


def extract_reasons(text):
    """Extract the invalidation grounds asserted by the requester."""
    # Try multiple anchors seen in real data
    patterns = [
        r'无效理由为[：:](.*?)(?=证据\d|经形式审查|提交的证据)',
        r'请求人提出的无效理由为[：:](.*?)(?=证据\d|经形式审查|提交的证据|请求人提交)',
        r'其理由是(.*?)(?=证据\d|经形式审查|同时提交)',
        r'理由是权利要求(.*?)(?=证据\d|同时提交|经形式)',
    ]
    for pat in patterns:
        m = re.search(pat, text, re.DOTALL)
        if m:
            reason = m.group(1).strip().replace('\n', ' ')
            if len(reason) > 10:
                return _truncate(reason, 400)
    return ''


def extract_decision_summary(text):
    """Extract the core reasoning / decision points from the decision section."""
    # Look for "决定的理由" or the final analytical section
    m = re.search(r'决定要点[：:](.*?)(?=附件|合议组|\|)', text, re.DOTALL)
    if m:
        return _truncate(m.group(1).strip().replace('\n', ' '), 400)

    # Fallback: last analytical paragraph before the table
    m = re.search(r'(综上所述.*?)(?=无效宣告请求人[：:]|附件|合议组|\Z)', text, re.DOTALL)
    if m:
        return _truncate(m.group(1).strip().replace('\n', ' '), 400)
    return ''


def extract_law_refs(text):
    """Extract cited patent-law articles from the full text."""
    refs = []
    # 第X条第Y款第Z项
    for m in re.finditer(r'专利法\s*第\s*(\d+)\s*条\s*第\s*(\d+)\s*款\s*第\s*(\d+)\s*项', text):
        refs.append(f'专利法第{m.group(1)}条第{m.group(2)}款第{m.group(3)}项')
    # 第X条第Y款
    for m in re.finditer(r'专利法\s*第\s*(\d+)\s*条\s*第\s*(\d+)\s*款', text):
        ref = f'专利法第{m.group(1)}条第{m.group(2)}款'
        if ref not in refs:
            refs.append(ref)
    # 第X条
    for m in re.finditer(r'专利法\s*第\s*(\d+)\s*条(?!\s*第)', text):
        ref = f'专利法第{m.group(1)}条'
        if ref not in refs:
            refs.append(ref)
    # 实施细则
    for m in re.finditer(r'实施细则\s*第\s*(\d+)\s*条\s*第\s*(\d+)\s*款', text):
        ref = f'实施细则第{m.group(1)}条第{m.group(2)}款'
        if ref not in refs:
            refs.append(ref)

    # Deduplicate preserving order, limit to 4
    seen = set()
    out = []
    for r in refs:
        if r not in seen:
            seen.add(r)
            out.append(r)
    return out[:4]


def _truncate(s, max_len):
    s = s.strip()
    if len(s) > max_len:
        return s[:max_len] + '...（略）'
    return s


def main():
    # Collect all MD files
    files = [f for f in os.listdir(DATA_DIR) if f.endswith('.md')]
    print(f"Found {len(files)} MD files", file=sys.stderr)

    random.seed(20241201)
    random.shuffle(files)

    # Parse and filter
    parsed = []
    skipped = {'no_text': 0, 'no_type': 0, 'no_verdict': 0, 'no_claims': 0}

    for fname in files:
        filepath = os.path.join(DATA_DIR, fname)
        text = read_text(filepath)
        if not text or len(text) < 500:
            skipped['no_text'] += 1
            continue

        ptype = classify_patent_type(text)
        if not ptype:
            skipped['no_type'] += 1
            continue

        verdict = extract_verdict(text)
        if not verdict:
            skipped['no_verdict'] += 1
            continue

        claims = extract_claims(text)
        if not claims:
            skipped['no_claims'] += 1
            # Still keep the case but with a note; claims is the most important field
            # Actually skip — without claims the case is as useless as old P2B
            continue

        patent_num = extract_patent_number(filepath, text)
        evidence = extract_evidence(text)
        reasons = extract_reasons(text)
        decision_summary = extract_decision_summary(text)
        law_refs = extract_law_refs(text)

        if not law_refs:
            # Fallback based on verdict
            if verdict == '全部无效':
                law_refs = ['专利法第22条第3款']
            elif verdict == '部分无效':
                law_refs = ['专利法第22条第3款', '专利法第46条第1款']
            else:
                law_refs = ['专利法第22条第3款']

        parsed.append({
            'patent_num': patent_num,
            'type': ptype,
            'verdict': verdict,
            'claims': claims,
            'evidence': evidence,
            'reasons': reasons,
            'decision_summary': decision_summary,
            'law_refs': law_refs,
        })

        if len(parsed) >= TARGET_COUNT * 3:  # over-sample for balanced selection
            break

    print(f"Parsed {len(parsed)} valid cases (skipped: {skipped})", file=sys.stderr)

    # Balanced selection: target equal verdict distribution
    # Dataset is ~30% 全部无效, ~34% 维持有效, ~4% 部分无效, ~32% unknown
    # Target: as balanced as possible given available data
    verdict_target = {
        '全部无效': TARGET_COUNT // 3,
        '维持有效': TARGET_COUNT // 3,
        '部分无效': TARGET_COUNT // 3,
    }

    selected = []
    verdict_counts = {'全部无效': 0, '维持有效': 0, '部分无效': 0}

    for p in parsed:
        v = p['verdict']
        if verdict_counts[v] < verdict_target[v]:
            selected.append(p)
            verdict_counts[v] += 1
        if sum(verdict_counts.values()) >= TARGET_COUNT:
            break

    # If 部分无效 is scarce, fill with others
    if len(selected) < TARGET_COUNT:
        for p in parsed:
            if p in selected:
                continue
            selected.append(p)
            if len(selected) >= TARGET_COUNT:
                break

    print(f"Selected {len(selected)} cases: {verdict_counts}", file=sys.stderr)

    # Build TestCase JSON
    cases = []
    for i, p in enumerate(selected[:TARGET_COUNT], start=1):
        case_id = f'invalidation_decision_{i:03d}'
        input_text = (
            f'无效宣告请求审查决定案例（{i}）。\n'
            f'涉案专利：{p["type"]}专利，专利号{p["patent_num"]}。\n'
            f'独立权利要求1：{p["claims"]}\n'
            f'主要证据：{p["evidence"] or "（未提取到）"}\n'
            f'请求理由：{p["reasons"] or "（未提取到）"}'
        )
        expected_text = (
            f'结论：{p["verdict"]}。\n'
            f'核心理由：{p["decision_summary"] or "（详见决定书正文）"}\n'
            f'主要法条：{"、".join(p["law_refs"])}。'
        )
        cases.append({
            'ID': case_id,
            'Domain': 'patent',
            'Input': input_text,
            'Expected': expected_text,
            'RequiredCitations': p['law_refs'],
        })

    with open(OUT_PATH, 'w', encoding='utf-8') as f:
        json.dump(cases, f, ensure_ascii=False, indent=2)
    print(f"Wrote {OUT_path if False else OUT_PATH} ({len(cases)} cases)", file=sys.stderr)

    # Print quality report
    _quality_report(cases)


def _quality_report(cases):
    """Print field completeness statistics."""
    print("\n=== 数据质量报告 ===", file=sys.stderr)
    total = len(cases)
    claims_filled = sum(1 for c in cases if '独立权利要求1：' in c['Input']
                        and len(c['Input'].split('独立权利要求1：')[1].split('\n')[0]) > 20)
    evidence_filled = sum(1 for c in cases if '主要证据：' in c['Input']
                          and '未提取到' not in c['Input'].split('主要证据：')[1].split('\n')[0])
    reasons_filled = sum(1 for c in cases if '请求理由：' in c['Input']
                         and '未提取到' not in c['Input'].split('请求理由：')[1].split('\n')[0])
    print(f"  总数: {total}", file=sys.stderr)
    print(f"  权利要求非空: {claims_filled}/{total} ({claims_filled*100//total}%)", file=sys.stderr)
    print(f"  证据非空: {evidence_filled}/{total} ({evidence_filled*100//total}%)", file=sys.stderr)
    print(f"  理由非空: {reasons_filled}/{total} ({reasons_filled*100//total}%)", file=sys.stderr)

    # Verdict distribution
    from collections import Counter
    verdicts = [c['Expected'].split('结论：')[1].split('。')[0] for c in cases]
    print(f"  结论分布: {dict(Counter(verdicts))}", file=sys.stderr)

    # Input length stats
    input_lens = [len(c['Input']) for c in cases]
    print(f"  Input 平均长度: {sum(input_lens)//total} 字符", file=sys.stderr)
    print(f"  Input 最短: {min(input_lens)}, 最长: {max(input_lens)}", file=sys.stderr)


if __name__ == '__main__':
    main()
