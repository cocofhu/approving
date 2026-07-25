#!/usr/bin/env node
/**
 * Build static site:
 * - copy site/ to public/
 * - convert content markdown files to public/<path>/index.html
 *
 * BASE_PATH defaults to / for the custom domain (www.approving-ai.com).
 * Legacy CI may set BASE_PATH=/approving-pages; that is remapped to / unless
 * FORCE_PROJECT_PAGES=1 (project Pages fallback).
 */
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import matter from "gray-matter";
import MarkdownIt from "markdown-it";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");
const siteDir = path.join(root, "site");
const contentDir = path.join(root, "content");
const outDir = path.join(root, "public");

// Custom domain is served from /. Legacy ci-docs set BASE_PATH=/approving-pages;
// remap that so CI builds root-relative assets (workflow env update needs workflow scope).
const envBase = process.env.BASE_PATH;
const useLegacyProjectPages =
  envBase === "/approving-pages" && process.env.FORCE_PROJECT_PAGES === "1";
const basePath = normalizeBase(
  useLegacyProjectPages ? envBase : (envBase === "/approving-pages" ? "/" : (envBase ?? "/")),
);
const md = new MarkdownIt({ html: false, linkify: true, typographer: true });

function normalizeBase(raw) {
  let b = String(raw || "/").trim();
  if (!b.startsWith("/")) b = `/${b}`;
  if (b !== "/" && b.endsWith("/")) b = b.slice(0, -1);
  return b;
}

function withBase(p) {
  if (!p) return basePath === "/" ? "/" : `${basePath}/`;
  if (/^https?:\/\//i.test(p) || p.startsWith("mailto:")) return p;
  const cleaned = p.startsWith("/") ? p : `/${p}`;
  if (basePath === "/") return cleaned;
  return `${basePath}${cleaned}`;
}

async function rmrf(dir) {
  await fs.rm(dir, { recursive: true, force: true });
}

async function ensureDir(dir) {
  await fs.mkdir(dir, { recursive: true });
}

async function copyDir(src, dest) {
  await ensureDir(dest);
  const entries = await fs.readdir(src, { withFileTypes: true });
  for (const entry of entries) {
    const from = path.join(src, entry.name);
    const to = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      await copyDir(from, to);
    } else {
      await ensureDir(path.dirname(to));
      let data = await fs.readFile(from);
      if (/\.(html|css|js)$/i.test(entry.name)) {
        data = Buffer.from(rewriteBasePlaceholders(data.toString("utf8")), "utf8");
      }
      await fs.writeFile(to, data);
    }
  }
}

function rewriteBasePlaceholders(text) {
  return text
    .replaceAll("{{BASE}}", basePath === "/" ? "" : basePath)
    .replaceAll("{{BASE_SLASH}}", basePath === "/" ? "/" : `${basePath}/`);
}

function docTemplate({ title, description, bodyHtml, relPath }) {
  const pageTitle = title ? `${title} · Approving` : "Approving";
  const desc = description || "Approving help documentation";
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(pageTitle)}</title>
  <meta name="description" content="${escapeHtml(desc)}">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Noto+Sans+SC:wght@400;500;600;700&family=Noto+Serif+SC:wght@500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="${withBase("/css/tokens.css")}">
  <link rel="stylesheet" href="${withBase("/css/site.css")}">
  <link rel="stylesheet" href="${withBase("/css/page.css")}">
</head>
<body class="page-doc">
  <header class="site-header">
    <div class="site-header__inner">
      <a class="site-header__brand" href="${withBase("/")}">Approving</a>
      <nav class="site-header__nav" aria-label="主导航">
        <a class="site-header__link" href="${withBase("/guide/quick-start/")}">文档</a>
        <a class="site-header__link" href="${withBase("/guide/concepts/")}">概念</a>
        <a class="site-header__link" href="https://github.com/cocofhu/approving" rel="noopener noreferrer" target="_blank">GitHub</a>
      </nav>
    </div>
  </header>
  <main id="main">
    <article class="doc">
      <header class="doc__header">
        <p class="doc__eyebrow">文档</p>
        <h1 class="doc__title">${escapeHtml(title || relPath)}</h1>
        ${description ? `<p class="doc__lead">${escapeHtml(description)}</p>` : ""}
      </header>
      <div class="doc__body">
        ${bodyHtml}
      </div>
    </article>
  </main>
  <footer class="site-footer">
    <div class="site-footer__inner">
      <p class="site-footer__brand">Approving</p>
      <nav class="site-footer__nav" aria-label="页脚">
        <a href="${withBase("/guide/quick-start/")}">快速开始</a>
        <a href="${withBase("/help/configuration/")}">配置</a>
        <a href="${withBase("/help/gateway/")}">网关</a>
        <a href="https://github.com/cocofhu/approving" rel="noopener noreferrer" target="_blank">源码</a>
      </nav>
      <p class="site-footer__meta">MIT · <a href="https://github.com/cocofhu/approving" rel="noopener noreferrer" target="_blank">cocofhu/approving</a></p>
    </div>
  </footer>
</body>
</html>
`;
}

function escapeHtml(s) {
  return String(s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function walkMarkdown(dir) {
  const out = [];
  async function walk(current) {
    const entries = await fs.readdir(current, { withFileTypes: true });
    for (const entry of entries) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) await walk(full);
      else if (entry.name.endsWith(".md")) out.push(full);
    }
  }
  await walk(dir);
  return out;
}

async function buildMarkdown() {
  const files = await walkMarkdown(contentDir);
  for (const file of files) {
    const raw = await fs.readFile(file, "utf8");
    const { data, content } = matter(raw);
    const rel = path.relative(contentDir, file).replace(/\\/g, "/");
    const slug = rel.replace(/\.md$/i, "");
    const outFile = path.join(outDir, slug, "index.html");
    const bodyHtml = md.render(content);
    const html = docTemplate({
      title: data.title || slug,
      description: data.description || "",
      bodyHtml,
      relPath: slug,
    });
    await ensureDir(path.dirname(outFile));
    await fs.writeFile(outFile, html, "utf8");
    console.log(`  md  ${slug}/`);
  }
}

async function main() {
  console.log(`Building Approving site (BASE_PATH=${basePath || "/"})`);
  await rmrf(outDir);
  await ensureDir(outDir);
  await copyDir(siteDir, outDir);
  await buildMarkdown();
  console.log(`Done → ${path.relative(root, outDir)}/`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
