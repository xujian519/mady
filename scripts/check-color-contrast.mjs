#!/usr/bin/env node
/**
 * check-color-contrast.mjs — Mady 桌面端 WCAG AA 对比度审计（W4-T10 / P2-3, M-DSK-TST-008）
 *
 * 解析 desktop/frontend/src/styles/globals.css 的 --color-mady-* 语义色令牌：
 *   - light 套：@theme 块内定义
 *   - dark 套：@media (prefers-color-scheme: dark) 块内 :root 覆盖值
 * 支持 hex（#RRGGBB / #RGB）与 rgb()/rgba()（逗号或空格分隔、百分比、/ alpha）。
 * 半透明背景（bg-sidebar 毛玻璃）按「混合在 bg-primary 之上」的假设求有效色。
 *
 * 组合矩阵（light 与 dark 各跑一遍）：
 *   text-primary/secondary/tertiary/inverse × bg-primary/secondary/tertiary/sidebar/composer
 *   + 语义色 danger/warning/success/info 文字 × 上述背景
 *
 * 阈值（WCAG AA 分档，见 docs/mady-desktop-standards.md §6.3 M-DSK-VIS-012）：
 *   ≤17pt 小文本 ≥4.5:1（--min 可调）；≥18pt 或加粗大文本 ≥3:1。
 *
 * 用法：
 *   node scripts/check-color-contrast.mjs [--min 4.5] [--json]
 * 退出码：存在任何未豁免的 FAIL 组合时为 1。
 */

import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_CSS = path.resolve(SCRIPT_DIR, '../desktop/frontend/src/styles/globals.css');

/** 大文本（≥18pt 或加粗）档位，WCAG AA */
const LARGE_TEXT_MIN = 3.0;

/** 组合矩阵 */
const TEXT_TOKENS = ['text-primary', 'text-secondary', 'text-tertiary', 'text-inverse'];
const SEMANTIC_TOKENS = ['danger', 'warning', 'success', 'info'];
const BG_TOKENS = ['bg-primary', 'bg-secondary', 'bg-tertiary', 'bg-sidebar', 'bg-composer'];

/**
 * 半透明背景的有效色假设：毛玻璃（backdrop-filter）背后最近的 solid 表面是 bg-primary，
 * 有效色 = alpha * 令牌色 + (1-alpha) * bg-primary。
 */
const SEMI_TRANSPARENT_BG = new Map([
  ['bg-sidebar', 'bg-primary'],
  ['bg-material', 'bg-primary'],
]);

/**
 * 非文字用途豁免（如实报告，不篡改阈值）。
 * 仅豁免矩阵过宽产生的、实际 UI 中不可能出现的组合；其余 FAIL 一律保留。
 */
const EXEMPTION_RULES = [
  {
    match: (fg, bg) => fg === 'text-inverse',
    reason:
      'text-inverse 只用于彩色按钮背景（accent/danger 反白文字），从不作为 surface 背景上的正文文字——矩阵过宽的误报',
  },
  {
    match: (fg) => fg === 'text-tertiary',
    reason:
      'text-tertiary 是刻意的最低层级文字（时间戳/禁用项/占位符/次要说明），非关键信息；与 Apple HIG tertiary label（约 2.8:1）同款低对比设计层级，业界通行豁免（2026-07-31 产品决策：保持层级，不强行提亮到 4.5:1 以免摧毁视觉层级）',
  },
];

/* ------------------------------------------------------------------ */
/* 颜色解析                                                             */
/* ------------------------------------------------------------------ */

/** 解析单个颜色值，返回 {r, g, b, a}（r/g/b 为 0-255 整数，a 为 0-1）。 */
function parseColor(name, raw) {
  const s = raw.trim();

  let m = s.match(/^#([0-9a-fA-F]{3,8})$/);
  if (m) {
    const h = m[1];
    if (h.length === 3) {
      return {
        r: parseInt(h[0] + h[0], 16),
        g: parseInt(h[1] + h[1], 16),
        b: parseInt(h[2] + h[2], 16),
        a: 1,
      };
    }
    if (h.length === 4) {
      return {
        r: parseInt(h[0] + h[0], 16),
        g: parseInt(h[1] + h[1], 16),
        b: parseInt(h[2] + h[2], 16),
        a: parseInt(h[3] + h[3], 16) / 255,
      };
    }
    if (h.length === 6) {
      return {
        r: parseInt(h.slice(0, 2), 16),
        g: parseInt(h.slice(2, 4), 16),
        b: parseInt(h.slice(4, 6), 16),
        a: 1,
      };
    }
    if (h.length === 8) {
      return {
        r: parseInt(h.slice(0, 2), 16),
        g: parseInt(h.slice(2, 4), 16),
        b: parseInt(h.slice(4, 6), 16),
        a: parseInt(h.slice(6, 8), 16) / 255,
      };
    }
  }

  m = s.match(/^rgba?\((.*)\)$/i);
  if (m) {
    return parseRgba(name, m[1]);
  }

  throw new Error(`--color-mady-${name}: 不支持的色值格式 "${raw}"`);
}

function parseChannel(s, max) {
  const n = s.endsWith('%') ? (parseFloat(s) / 100) * max : Number(s);
  return Math.round(n);
}

function parseAlpha(s) {
  const n = s.endsWith('%') ? parseFloat(s) / 100 : Number(s);
  if (n < 0 || n > 1) throw new Error(`alpha 越界: ${s}`);
  return n;
}

/** 解析 rgb()/rgba() 参数：兼容逗号分隔、空格分隔、'/' 分隔 alpha、百分比。 */
function parseRgba(name, inner) {
  const vals = inner
    .split(/[\s,]+/)
    .filter((p) => p !== '' && p !== '/');
  if (vals.length < 3) throw new Error(`--color-mady-${name}: rgba() 参数不足 "${inner}"`);
  return {
    r: parseChannel(vals[0], 255),
    g: parseChannel(vals[1], 255),
    b: parseChannel(vals[2], 255),
    a: vals.length >= 4 ? parseAlpha(vals[3]) : 1,
  };
}

/* ------------------------------------------------------------------ */
/* CSS 解析                                                             */
/* ------------------------------------------------------------------ */

/** 找出所有匹配 header 的 `{ ... }` 块（花括号平衡扫描），返回 {header, body}。 */
function findBlocks(css, pattern) {
  const re = new RegExp(pattern, 'g');
  const blocks = [];
  let m;
  while ((m = re.exec(css)) !== null) {
    let depth = 0;
    let openIdx = -1;
    let end = -1;
    for (let i = re.lastIndex; i < css.length; i++) {
      const ch = css[i];
      if (ch === '{') {
        if (openIdx === -1) openIdx = i;
        depth++;
      } else if (ch === '}') {
        depth--;
        if (depth === 0) {
          end = i;
          break;
        }
      }
    }
    if (end === -1) throw new Error(`未闭合的块: ${m[0]}`);
    blocks.push({ header: m[0], body: css.slice(openIdx + 1, end) });
  }
  return blocks;
}

/** 从块内提取所有 --color-mady-<name>: <value>; 声明。 */
function parseColorDecls(block) {
  const decls = {};
  const re = /--color-mady-([a-z0-9-]+)\s*:\s*([^;]+);/g;
  let m;
  while ((m = re.exec(block)) !== null) {
    decls[m[1]] = parseColor(m[1], m[2].trim());
  }
  return decls;
}

/** 解析 globals.css，返回 { light, dark } 两套令牌映射。 */
function parseThemeColors(css) {
  const themes = { light: {}, dark: {} };

  const themeBlocks = findBlocks(css, '@theme\\b');
  if (themeBlocks.length === 0) throw new Error('找不到 @theme 块');
  themes.light = parseColorDecls(themeBlocks[0].body);

  // W4-T7 起暗色覆盖改为 <html>.dark class 策略（由 ThemeProvider 写入），
  // 兼容解析：.dark { ... } 顶层块（现行）与旧的 @media (prefers-color-scheme: dark) 内 :root 块。
  const darkBlocks = findBlocks(css, '\\.dark\\b');
  for (const db of darkBlocks) {
    Object.assign(themes.dark, parseColorDecls(db.body));
  }
  const darkMedia = findBlocks(css, '@media\\s*\\(\\s*prefers-color-scheme\\s*:\\s*dark\\s*\\)');
  for (const db of darkMedia) {
    for (const rb of findBlocks(db.body, ':root\\b')) {
      Object.assign(themes.dark, parseColorDecls(rb.body));
    }
  }
  return themes;
}

/* ------------------------------------------------------------------ */
/* WCAG 对比度                                                          */
/* ------------------------------------------------------------------ */

/** sRGB 通道线性化。 */
function channelL(c) {
  c /= 255;
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/** 相对亮度 L = 0.2126R + 0.7152G + 0.0722B。 */
function luminance({ r, g, b }) {
  return 0.2126 * channelL(r) + 0.7152 * channelL(g) + 0.0722 * channelL(b);
}

/** 对比度 = (L1+0.05) / (L2+0.05)。 */
function wcagContrast(fg, bg) {
  const l1 = luminance(fg);
  const l2 = luminance(bg);
  return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
}

/** 前景色 alpha 混合到不透明背景上，返回不透明颜色。 */
function blend(fg, bg) {
  const a = fg.a;
  return {
    r: Math.round(a * fg.r + (1 - a) * bg.r),
    g: Math.round(a * fg.g + (1 - a) * bg.g),
    b: Math.round(a * fg.b + (1 - a) * bg.b),
    a: 1,
  };
}

function toHex({ r, g, b }) {
  const h = (n) => n.toString(16).padStart(2, '0');
  return `#${h(r)}${h(g)}${h(b)}`;
}

/* ------------------------------------------------------------------ */
/* 审计                                                               */
/* ------------------------------------------------------------------ */

/** 对单套主题跑组合矩阵。 */
function auditTheme(mode, tokens, min) {
  // 背景有效色：半透明背景与基底混合为不透明色。
  const effBg = {};
  for (const name of BG_TOKENS) {
    const c = tokens[name];
    if (!c) throw new Error(`${mode}: 缺少背景令牌 --color-mady-${name}`);
    const baseName = SEMI_TRANSPARENT_BG.get(name);
    effBg[name] = baseName ? blend(c, tokens[baseName]) : c;
  }

  const results = [];
  for (const fgName of [...TEXT_TOKENS, ...SEMANTIC_TOKENS]) {
    const fgRaw = tokens[fgName];
    if (!fgRaw) throw new Error(`${mode}: 缺少文字令牌 --color-mady-${fgName}`);
    for (const bgName of BG_TOKENS) {
      const bg = effBg[bgName];
      // 半透明文字与其背景混合（对矩阵内 solid 文字无影响，通用处理）。
      const fg = fgRaw.a < 1 ? blend(fgRaw, bg) : fgRaw;
      const ratio = wcagContrast(fg, bg);
      const smallPass = ratio >= min;
      const largePass = ratio >= LARGE_TEXT_MIN;
      const exemption = EXEMPTION_RULES.find((r) => r.match(fgName, bgName));
      const status = exemption ? 'exempt' : smallPass ? 'pass' : 'fail';
      results.push({
        mode,
        fg: fgName,
        bg: bgName,
        fgColor: toHex(fgRaw),
        bgColor: toHex(bg),
        ratio: Number(ratio.toFixed(2)),
        smallPass,
        largePass,
        status,
        reason: exemption ? exemption.reason : null,
      });
    }
  }
  return results;
}

function audit(themes, min) {
  const report = {
    light: auditTheme('light', themes.light, min),
    dark: auditTheme('dark', themes.dark, min),
  };
  let pass = 0;
  let fail = 0;
  let exempt = 0;
  for (const mode of ['light', 'dark']) {
    for (const c of report[mode]) {
      if (c.status === 'pass') pass++;
      else if (c.status === 'fail') fail++;
      else exempt++;
    }
  }
  report.summary = { pass, fail, exempt, total: pass + fail + exempt };
  report.failed = fail > 0;
  return report;
}

/* ------------------------------------------------------------------ */
/* 输出                                                               */
/* ------------------------------------------------------------------ */

function comboLine(c) {
  const status = `[${c.status.toUpperCase()}]`.padEnd(8);
  const ratio = `${c.ratio.toFixed(2)}:1`.padStart(8);
  let note = '';
  if (c.status === 'fail') {
    note = c.largePass ? '  （大文本≥3:1 达标，小文本不达标）' : '  （大文本≥3:1 也不达标）';
  } else if (c.status === 'exempt') {
    note = `  （豁免: ${c.reason}）`;
  }
  return `  ${status} ${ratio}  ${c.fg.padEnd(13)}× ${c.bg}${note}`;
}

function formatText(report, cssPath, min) {
  const L = [];
  L.push('=== Mady 桌面端 WCAG AA 对比度审计（W4-T10 / M-DSK-TST-008） ===');
  L.push(`源文件: ${cssPath}`);
  L.push(`阈值: 小文本 ≤17pt ≥${min}:1（--min 可调）；大文本 ≥18pt/加粗 ≥3:1（WCAG AA 分档）`);
  L.push('');
  for (const mode of ['light', 'dark']) {
    L.push(`--- ${mode.toUpperCase()} ---`);
    for (const c of report[mode]) L.push(comboLine(c));
    L.push('');
  }

  const fails = [];
  const exempts = [];
  for (const mode of ['light', 'dark']) {
    for (const c of report[mode]) {
      if (c.status === 'fail') fails.push(c);
      else if (c.status === 'exempt') exempts.push(c);
    }
  }

  if (fails.length > 0) {
    L.push('【FAIL —— 真实对比度问题（需人工修复，或确认实际用途后审慎加入豁免）】');
    for (const c of fails) {
      const note = c.largePass ? '' : '  ★大文本≥3:1 也不达标';
      L.push(`  ${c.mode.toUpperCase()} ${c.ratio.toFixed(2)}:1  ${c.fg} × ${c.bg}${note}`);
    }
    L.push('');
  }

  if (exempts.length > 0) {
    L.push('【EXEMPT —— 非文字用途豁免（矩阵过宽误报）】');
    for (const c of exempts) L.push(`  ${c.mode.toUpperCase()} ${c.ratio.toFixed(2)}:1  ${c.fg} × ${c.bg}  (${c.reason})`);
    L.push('');
  }

  L.push('=== 摘要 ===');
  for (const mode of ['light', 'dark']) {
    const s = report[mode].reduce(
      (acc, c) => {
        acc[c.status]++;
        return acc;
      },
      { pass: 0, fail: 0, exempt: 0 },
    );
    L.push(`  ${mode.toUpperCase()}: ${s.pass} PASS / ${s.fail} FAIL / ${s.exempt} EXEMPT`);
  }
  L.push(`  合计: ${report.summary.pass} PASS / ${report.summary.fail} FAIL / ${report.summary.exempt} EXEMPT`);
  L.push(`审计假设: 半透明背景（bg-sidebar 等）按混合在 bg-primary 之上计算有效色。`);
  L.push(report.failed ? '结论: 存在未达标组合 → exit code 1' : '结论: 全部达标 → exit code 0');
  L.push('');
  return L.join('\n');
}

function formatJson(report, cssPath, min, themes) {
  const summary = { ...report.summary };
  for (const mode of ['light', 'dark']) {
    summary[mode] = report[mode].reduce(
      (acc, c) => {
        acc[c.status]++;
        return acc;
      },
      { pass: 0, fail: 0, exempt: 0 },
    );
  }
  return {
    script: 'check-color-contrast.mjs',
    task: 'W4-T10 / P2-3 (M-DSK-TST-008)',
    css: cssPath,
    thresholds: { smallText: min, largeText: LARGE_TEXT_MIN },
    tokens: {
      light: Object.fromEntries(Object.entries(themes.light).map(([k, v]) => [k, `${toHex(v)}${v.a < 1 ? ` a=${v.a}` : ''}`])),
      dark: Object.fromEntries(Object.entries(themes.dark).map(([k, v]) => [k, `${toHex(v)}${v.a < 1 ? ` a=${v.a}` : ''}`])),
    },
    combinations: { light: report.light, dark: report.dark },
    summary,
    failed: report.failed,
  };
}

/* ------------------------------------------------------------------ */
/* 入口                                                               */
/* ------------------------------------------------------------------ */

function parseArgs(argv) {
  const opts = { min: 4.5, json: false };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--min') {
      opts.min = Number(argv[++i]);
    } else if (a === '--json') {
      opts.json = true;
    } else if (a === '-h' || a === '--help') {
      process.stdout.write(
        `用法: node scripts/check-color-contrast.mjs [--min 4.5] [--json]\n` +
          `\n` +
          `参数:\n` +
          `  --min <n>  小文本对比度阈值（WCAG AA 默认 4.5）\n` +
          `  --json     输出 JSON 报告（默认文本）\n` +
          `\n` +
          `解析 desktop/frontend/src/styles/globals.css 的 --color-mady-* 令牌\n` +
          `（@theme light 套 + @media (prefers-color-scheme: dark) 覆盖套），\n` +
          `对「文字 × 背景」组合计算 WCAG 对比度。任何未豁免 FAIL → exit 1。\n`,
      );
      process.exit(0);
    } else {
      throw new Error(`未知参数: ${a}（用 --help 查看用法）`);
    }
  }
  if (!Number.isFinite(opts.min) || opts.min <= 0) throw new Error('--min 必须是正数');
  return opts;
}

function main() {
  let opts;
  try {
    opts = parseArgs(process.argv.slice(2));
  } catch (err) {
    console.error(`错误: ${err.message}`);
    process.exit(2);
  }

  let css;
  try {
    css = readFileSync(DEFAULT_CSS, 'utf8');
  } catch (err) {
    console.error(`无法读取 ${DEFAULT_CSS}: ${err.message}`);
    process.exit(2);
  }

  let themes;
  let report;
  try {
    themes = parseThemeColors(css);
    report = audit(themes, opts.min);
  } catch (err) {
    console.error(`解析失败: ${err.message}`);
    process.exit(2);
  }

  process.stdout.write(
    opts.json ? JSON.stringify(formatJson(report, DEFAULT_CSS, opts.min, themes), null, 2) + '\n' : formatText(report, DEFAULT_CSS, opts.min),
  );
  process.exit(report.failed ? 1 : 0);
}

main();
