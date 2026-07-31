async function(args) {
  const link = document.querySelector('a.pdfLink, a[href*=".pdf"]');
  if (link) return link.href;
  const meta = document.querySelector('meta[name="citation_pdf_url"]');
  return meta ? meta.content : '';
}
