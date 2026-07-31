function boundedInteger(value, fallback, max) {
  const number = value === undefined ? fallback : Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.max(1, Math.min(max, Math.trunc(number)));
}

function parseMetadataLine(line) {
  // Format: "CN • CN110515732B • 马惠敏 • 清华大学"
  const parts = line.split("•").map((s) => s.trim()).filter(Boolean);
  return {
    country: parts[0] || "",
    number: parts[1] || "",
    inventors: parts[2] || "",
    assignee: parts.slice(3).join(", ") || "",
  };
}

function parseDateLine(line) {
  // Format: "Priority 2016-10-31 • Filed 2016-10-31 • Granted 2019-12-24 • Published 2019-12-24"
  const dates = {};
  for (const part of line.split("•")) {
    const m = part.trim().match(/^([A-Za-z]+)\s+(\d{4}-\d{2}-\d{2})$/);
    if (m) dates[m[1].toLowerCase()] = m[2];
  }
  return dates;
}

export async function searchPatents(ctx, args = {}) {
  const query = args.query || "";
  const maxResults = boundedInteger(args.maxResults, 10, 100);
  if (!query) throw new Error("search query is required");

  let q = query;
  if (args.country) {
    const hasCountry = new RegExp(`country\\s*:\\s*${args.country}`, "i").test(q);
    if (!hasCountry) q += ` country:${args.country}`;
  }

  const url = `https://patents.google.com/?q=${encodeURIComponent(q)}&num=${maxResults}`;
  await ctx.openOrReuseTab(url, { wait: true, timeout: 30 });
  await ctx.waitForLoad();
  await ctx.wait(2);

  const results = await ctx.js(String.raw`(() => {
    const items = [...document.querySelectorAll('search-result-item')]
    return items.slice(0, ${maxResults}).map(item => {
      const lines = item.innerText.split('\n').map(l => l.trim()).filter(Boolean)
      const title = lines[0] || ''
      const metaLine = lines[1] || ''
      const dateLine = lines[2] || ''
      const abstract = lines.slice(3).join('\n').slice(0, 500)
      const pdf = item.querySelector('a.pdfLink')
      const link = item.querySelector('a#link')
      return {
        title,
        metaLine,
        dateLine,
        abstract,
        number: pdf ? pdf.innerText.trim() : '',
        pdfUrl: pdf ? pdf.href : '',
        url: link ? link.href : '',
        itemId: item.id,
      }
    }).filter(r => r.title)
  })()`);

  return results.map((r) => {
    const meta = parseMetadataLine(r.metaLine);
    return {
      title: r.title,
      number: meta.number || r.number,
      country: meta.country,
      inventors: meta.inventors,
      assignee: meta.assignee,
      dates: parseDateLine(r.dateLine),
      abstract: r.abstract,
      url: r.itemId ? `https://patents.google.com/${r.itemId}` : r.url,
      pdfUrl: r.pdfUrl,
    };
  });
}
