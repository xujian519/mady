function boundedInteger(value, fallback, max) {
  const number = value === undefined ? fallback : Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.max(1, Math.min(max, Math.trunc(number)));
}

function parsePubNumber(raw) {
  // "CN107891199A (B)" → { number: "CN107891199A", family: ["A", "B"] }
  const m = raw.trim().match(/^([A-Z]{2}\d+)(?:\s*\(([^)]+)\))?$/);
  if (!m) return { number: raw.trim(), kinds: [] };
  return { number: m[1], kinds: m[2] ? m[2].split(/\s*[,/]\s*/) : [] };
}

export async function searchPatents(ctx, args = {}) {
  const query = args.query || "";
  const maxResults = boundedInteger(args.maxResults, 10, 50);
  if (!query) throw new Error("search query is required");

  const url = `https://worldwide.espacenet.com/patent/search?q=${encodeURIComponent(query)}`;
  await ctx.openOrReuseTab(url, { wait: true, timeout: 30 });
  await ctx.waitForLoad();
  await ctx.wait(3);

  // 结果通过 XHR 异步加载，滚动触发渲染。
  await ctx.scrollToBottomUntil(
    async () => {
      const n = await ctx.js(String.raw`document.querySelectorAll('article[data-qa="result_resultList"]').length`);
      return n >= 1;
    },
    { step: 600, wait: 1, maxSteps: 10 }
  );

  const results = await ctx.js(String.raw`(() => {
    const items = [...document.querySelectorAll('article[data-qa="result_resultList"]')]
    return items.slice(0, ${maxResults}).map(item => {
      const title = item.querySelector('span[lang="en"]')?.innerText?.trim() || ''
      const subtitle = item.querySelector('[class*="subtitle"]')?.innerText?.trim() || ''
      const applicant = item.querySelector('[title="Applicant"] span')?.innerText?.trim() || ''
      const abstract = item.querySelector('[class*="abstract"]')?.innerText?.trim() || ''
      return { title, subtitle, applicant, abstract }
    }).filter(r => r.title)
  })()`);

  return results.map((r) => {
    const parts = r.subtitle.split("•").map((s) => s.trim()).filter(Boolean);
    const pub = parsePubNumber(parts[0] || "");
    return {
      title: r.title,
      number: pub.number,
      kinds: pub.kinds,
      date: parts[1] || "",
      applicant: r.applicant,
      abstract: r.abstract.slice(0, 500),
      url: pub.number ? `https://worldwide.espacenet.com/publicationDetails/biblio?CC=${pub.number.slice(0, 2)}&NR=${pub.number.slice(2)}&KC=${pub.kinds[0] || "A"}` : "",
    };
  });
}
