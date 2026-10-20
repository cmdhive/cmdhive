// Renders brand images from HTML/SVG in headless Chrome (exact pixels, real fonts).
//   node scripts/render.cjs <input.svg|input.html> <out.png> <width> <height>
// Needs Playwright (the same one the UI test uses).
const { chromium } = require("playwright");
const fs = require("fs");
const path = require("path");

(async () => {
  const [input, out, w, h] = process.argv.slice(2);
  const width = +w, height = +(h || w);
  const src = fs.readFileSync(input, "utf8");
  const html = input.endsWith(".svg")
    ? `<html><body style="margin:0;background:transparent">${src.replace("<svg ", `<svg width="${width}" height="${height}" `)}</body></html>`
    : src;
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: 1 });
    // A page is opened from its file, so its fonts and images load by relative path.
    if (input.endsWith(".svg")) await page.setContent(html, { waitUntil: "networkidle" });
    else await page.goto("file:///" + path.resolve(input).replace(/\\/g, "/"), { waitUntil: "networkidle" });
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: out, omitBackground: true, clip: { x: 0, y: 0, width, height } });
    console.log(`${path.basename(out)} ${width}x${height}`);
  } finally {
    await browser.close();
  }
})().catch((e) => { console.error(e); process.exit(1); });
