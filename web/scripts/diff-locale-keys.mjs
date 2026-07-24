#!/usr/bin/env node
/**
 * Compare flattened key paths between zh-CN and en locale directories.
 * Reports keys present in one locale but missing in the other (per JSON file).
 */
import { readdir, readFile } from 'node:fs/promises'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const localesRoot = join(__dirname, '../src/locales')
const LOCALES = ['zh-CN', 'en']

/** @param {unknown} obj @param {string} [prefix] @returns {string[]} */
function flattenKeys(obj, prefix = '') {
  if (obj == null || typeof obj !== 'object' || Array.isArray(obj)) {
    return prefix ? [prefix] : []
  }
  const keys = []
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (v != null && typeof v === 'object' && !Array.isArray(v)) {
      keys.push(...flattenKeys(v, path))
    } else {
      keys.push(path)
    }
  }
  return keys
}

/** @param {string} locale */
async function listJsonFiles(locale) {
  const dir = join(localesRoot, locale)
  const entries = await readdir(dir, { withFileTypes: true })
  return entries.filter((e) => e.isFile() && e.name.endsWith('.json')).map((e) => e.name).sort()
}

/** @param {string} locale @param {string} file */
async function loadKeys(locale, file) {
  const raw = await readFile(join(localesRoot, locale, file), 'utf8')
  const parsed = JSON.parse(raw)
  return new Set(flattenKeys(parsed))
}

function reportMissing(label, missing) {
  if (!missing.length) return 0
  console.log(`\n${label} (${missing.length}):`)
  for (const k of missing) console.log(`  - ${k}`)
  return missing.length
}

async function main() {
  const [zhFiles, enFiles] = await Promise.all(LOCALES.map(listJsonFiles))
  const allFiles = [...new Set([...zhFiles, ...enFiles])].sort()

  let totalMissing = 0
  let hasDiff = false

  console.log('Locale key diff (zh-CN vs en)')
  console.log('='.repeat(40))

  for (const file of allFiles) {
    const inZh = zhFiles.includes(file)
    const inEn = enFiles.includes(file)

    if (!inZh || !inEn) {
      hasDiff = true
      if (!inZh) console.log(`\n[${file}] missing in zh-CN`)
      if (!inEn) console.log(`\n[${file}] missing in en`)
      totalMissing++
      continue
    }

    const [zhKeys, enKeys] = await Promise.all([loadKeys('zh-CN', file), loadKeys('en', file)])
    const onlyZh = [...zhKeys].filter((k) => !enKeys.has(k)).sort()
    const onlyEn = [...enKeys].filter((k) => !zhKeys.has(k)).sort()

    if (onlyZh.length || onlyEn.length) {
      hasDiff = true
      console.log(`\n[${file}]`)
      totalMissing += reportMissing('  only in zh-CN', onlyZh)
      totalMissing += reportMissing('  only in en', onlyEn)
    }
  }

  if (!hasDiff) {
    console.log('\nAll locale files and keys are in sync.')
    process.exit(0)
  }

  console.log(`\nTotal missing key paths: ${totalMissing}`)
  process.exit(1)
}

main().catch((err) => {
  console.error(err)
  process.exit(2)
})
