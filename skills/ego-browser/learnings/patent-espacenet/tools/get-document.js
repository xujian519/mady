function toBiblioURL(patentNumber) {
  const clean = patentNumber.trim();
  if (/^https?:\/\//.test(clean)) return clean;
  const m = clean.match(/^([A-Z]{2})(\d+)([A-Z]\d?)?$/i);
  if (!m) throw new Error(`invalid publication number: ${patentNumber}`);
  const cc = m[1].toUpperCase();
  const nr = m[2];
  const kc = (m[3] || "A").toUpperCase();
  return `https://worldwide.espacenet.com/publicationDetails/biblio?CC=${cc}&NR=${nr}&KC=${kc}`;
}

export async function getPatentDocument(ctx, args = {}) {
  const patentNumber = args.patentNumber || "";
  if (!patentNumber) throw new Error("patent number is required");

  const url = toBiblioURL(patentNumber);
  await ctx.openOrReuseTab(url, { wait: true, timeout: 30 });
  await ctx.waitForLoad();
  await ctx.wait(3);

  const doc = await ctx.js(String.raw`(() => {
    const title = document.querySelector('[data-qa="biblio_title"]')?.innerText?.trim()
      || document.querySelector('h1')?.innerText?.trim() || ''
    const number = document.querySelector('[data-qa="publicationNumber"]')?.innerText?.trim() || ''
    const applicant = document.querySelector('[data-qa="applicant"]')?.innerText?.trim() || ''
    const inventor = document.querySelector('[data-qa="inventor"]')?.innerText?.trim() || ''
    const abstract = document.querySelector('[data-qa="abstract"]')?.innerText?.trim() || ''
    return { title, number, applicant, inventor, abstract }
  })()`);

  return {
    number: doc.number || patentNumber,
    title: doc.title,
    applicant: doc.applicant,
    inventor: doc.inventor,
    abstract: doc.abstract,
    url,
  };
}
