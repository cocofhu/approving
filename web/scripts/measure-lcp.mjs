import { chromium } from 'playwright';

const url = process.argv[2] || 'http://127.0.0.1:4173/login';
const runs = Number(process.argv[3] || 3);

const slow4g = {
  offline: false,
  downloadThroughput: (1.6 * 1024 * 1024) / 8,
  uploadThroughput: (750 * 1024) / 8,
  latency: 150,
};

async function once() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1280, height: 720 },
  });
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send('Network.enable');
  await cdp.send('Network.emulateNetworkConditions', {
    offline: slow4g.offline,
    latency: slow4g.latency,
    downloadThroughput: slow4g.downloadThroughput,
    uploadThroughput: slow4g.uploadThroughput,
    connectionType: 'cellular3g',
  });
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });

  await page.addInitScript(() => {
    window.__cwv = { lcp: null, cls: 0, lcpEntry: null };
    new PerformanceObserver((list) => {
      for (const e of list.getEntries()) {
        window.__cwv.lcp = e.startTime;
        window.__cwv.lcpEntry = {
          startTime: e.startTime,
          size: e.size,
          url: e.url || '',
          id: e.id,
          tag: e.element?.tagName,
          className: e.element?.className,
          text: e.element?.textContent?.slice(0, 40),
        };
      }
    }).observe({ type: 'largest-contentful-paint', buffered: true });
    new PerformanceObserver((list) => {
      for (const e of list.getEntries()) {
        if (!e.hadRecentInput) window.__cwv.cls += e.value;
      }
    }).observe({ type: 'layout-shift', buffered: true });
  });

  await page.goto(url, { waitUntil: 'networkidle', timeout: 120000 });
  await page.waitForTimeout(2500);

  const result = await page.evaluate(() => {
    const nav = performance.getEntriesByType('navigation')[0];
    const resources = performance.getEntriesByType('resource').map((r) => ({
      name: r.name,
      initiatorType: r.initiatorType,
      duration: Math.round(r.duration),
      start: Math.round(r.startTime),
      transferSize: r.transferSize,
      encodedBodySize: r.encodedBodySize,
      responseEnd: Math.round(r.responseEnd),
    }));
    const fonts = resources.filter((r) =>
      /font|woff|googleapis|gstatic|Inter|JetBrains/i.test(r.name),
    );
    const paint = performance.getEntriesByType('paint');
    return {
      cwv: window.__cwv,
      ttfb: nav?.responseStart,
      fcp: paint.find((p) => p.name === 'first-contentful-paint')?.startTime,
      fonts,
      topSlow: [...resources].sort((a, b) => b.duration - a.duration).slice(0, 12),
    };
  });

  await browser.close();
  return result;
}

const all = [];
for (let i = 0; i < runs; i++) {
  console.error(`run ${i + 1}/${runs}...`);
  const r = await once();
  all.push(r);
  console.log(
    JSON.stringify(
      {
        run: i + 1,
        lcp_ms: Math.round(r.cwv.lcp),
        cls: Number(r.cwv.cls.toFixed(4)),
        lcpElement: r.cwv.lcpEntry,
        ttfb_ms: Math.round(r.ttfb),
        fcp_ms: Math.round(r.fcp || 0),
        fonts: r.fonts,
        topSlow: r.topSlow,
      },
      null,
      2,
    ),
  );
}

const lcps = all.map((a) => a.cwv.lcp).sort((a, b) => a - b);
const mid = lcps[Math.floor(lcps.length / 2)];
console.log('\nMEDIAN_LCP_MS', Math.round(mid));
console.log(
  'CLS_VALUES',
  all.map((a) => Number(a.cwv.cls.toFixed(4))),
);
