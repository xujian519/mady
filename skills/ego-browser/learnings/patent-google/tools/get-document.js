function toPatentURL(patentNumber) {
  if (/^https?:\/\//.test(patentNumber)) return patentNumber;
  const clean = patentNumber.trim().replace(/[^A-Za-z0-9]/g, "");
  if (!clean) throw new Error("patent number is required");
  return `https://patents.google.com/patent/${clean}/zh`;
}

export async function getPatentDocument(ctx, args = {}) {
  const patentNumber = args.patentNumber || "";
  if (!patentNumber) throw new Error("patent number is required");

  const url = toPatentURL(patentNumber);
  await ctx.openOrReuseTab(url, { wait: true, timeout: 30 });
  await ctx.waitForLoad();
  await ctx.wait(2);

  // Claims 与 description 为懒加载：滚动到底部触发渲染后重新提取。
  await ctx.scrollToBottomUntil(
    async () => {
      const n = await ctx.js(String.raw`document.querySelectorAll('section#claims, section#description').length`);
      return n >= 2;
    },
    { step: 800, wait: 1, maxSteps: 15 }
  );

  const doc = await ctx.js(String.raw`(() => {
    const title = document.querySelector('meta[name="DC.title"]')?.content?.trim() || ''
    const number = document.querySelector('meta[name="citation_patent_number"]')?.content?.trim() || ''
    const abstract = document.querySelector('section#abstract')?.innerText?.trim() || ''
    const claims = document.querySelector('section#claims')?.innerText?.trim() || ''
    const description = document.querySelector('section#description')?.innerText?.trim() || ''
    return { title, number, abstract, claims, description }
  })()`);

  return {
    title: doc.title,
    number: doc.number || patentNumber,
    abstract: doc.abstract,
    claims: doc.claims,
    description: doc.description,
    url,
  };
}
