import { chromium } from 'playwright';

const url = process.argv[2] || 'http://127.0.0.1:4173/login';
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport: { width: 1365, height: 900 } });
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Network.enable');
await cdp.send('Network.emulateNetworkConditions', {
  offline: false,
  latency: 150,
  downloadThroughput: (1.6 * 1024 * 1024) / 8,
  uploadThroughput: (750 * 1024) / 8,
  connectionType: 'cellular3g',
});
await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });

await page.addInitScript(() => {
  window.__lcps = [];
  new PerformanceObserver((list) => {
    for (const e of list.getEntries()) {
      window.__lcps.push({
        t: e.startTime,
        size: e.size,
        url: e.url || '',
        tag: e.element?.tagName,
        className: e.element?.className,
        text: e.element?.textContent?.slice(0, 60),
      });
    }
  }).observe({ type: 'largest-contentful-paint', buffered: true });
});

await page.goto(url, { waitUntil: 'networkidle', timeout: 120000 });
await page.waitForTimeout(4000);
const brand = await page.$('.brand-logo__name');
const box = brand ? await brand.boundingBox() : null;
const style = brand
  ? await brand.evaluate((el) => {
      const s = getComputedStyle(el);
      return {
        color: s.color,
        fill: s.webkitTextFillColor,
        fontFamily: s.fontFamily,
        fontWeight: s.fontWeight,
        fontSize: s.fontSize,
        bgClip: s.backgroundClip || s.webkitBackgroundClip,
      };
    })
  : null;
const data = await page.evaluate(() => {
  const resources = performance.getEntriesByType('resource').map((r) => ({
    name: r.name,
    dur: Math.round(r.duration),
    start: Math.round(r.startTime),
    end: Math.round(r.responseEnd),
    type: r.initiatorType,
  }));
  const fonts = resources.filter((r) => /woff|gstatic|font/i.test(r.name));
  return { lcps: window.__lcps, fonts, blocking: resources.filter(r => /googleapis|fonts\.|index-.*\.css/.test(r.name)) };
});
console.log(JSON.stringify({ url, box, style, ...data }, null, 2));
await browser.close();
