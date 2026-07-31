import { writeFile } from "node:fs/promises";
import { mkdir } from "node:fs/promises";
import path from "node:path";

function toPatentURL(patentNumber) {
  if (/^https?:\/\//.test(patentNumber)) return patentNumber;
  const clean = patentNumber.trim().replace(/[^A-Za-z0-9]/g, "");
  if (!clean) throw new Error("patent number is required");
  return `https://patents.google.com/patent/${clean}/zh`;
}

// serverFetch 从 Node 侧发起请求，可直接下载 PDF 字节流。
export async function downloadPatentPDF(ctx, args = {}) {
  const patentNumber = args.patentNumber || "";
  const outputDir = args.outputDir || process.cwd();
  if (!patentNumber) throw new Error("patent number is required");

  const clean = patentNumber.trim().replace(/[^A-Za-z0-9]/g, "");
  const url = toPatentURL(patentNumber);
  await ctx.openOrReuseTab(url, { wait: true, timeout: 30 });
  await ctx.waitForLoad();
  await ctx.wait(2);

  const pdfUrl = await ctx.js(String.raw`(() => {
    const link = document.querySelector('a.pdfLink, a[href*=".pdf"]')
    const meta = document.querySelector('meta[name="citation_pdf_url"]')
    return link ? link.href : meta ? meta.content : ''
  })()`);

  if (!pdfUrl) {
    return { patentNumber: clean, success: false, error: "PDF link not found on detail page" };
  }

  await mkdir(outputDir, { recursive: true });
  const file = path.join(outputDir, `${clean}.pdf`);
  const data = await ctx.serverFetch(pdfUrl);
  await writeFile(file, data);

  return { patentNumber: clean, file, success: true };
}
