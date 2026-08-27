import { readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const output = resolve(process.cwd(), process.argv[2] ?? '.output/public')
const path = resolve(output, '404.html')
const base = process.env.NUXT_APP_BASE_URL ?? '/'
if (!/^\/(?:[A-Za-z0-9._~-]+\/)*$/.test(base)) throw new Error(`unsafe Pages base URL: ${base}`)

let html = readFileSync(path, 'utf8')
html = html.replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
const shell = '<div id="__nuxt"></div>'
const page = `<div id="__nuxt"><main><section class="error-page container"><p class="eyebrow">404</p><h1>Page not found</h1><p>The page may have moved or the generated directory does not contain this entry.</p><a class="button button--primary" href="${base}plugins">Browse plugins</a></section></main></div>`
if (!html.includes(shell)) throw new Error('generated 404 shell is missing')
html = html.replace(shell, page)
writeFileSync(path, html)
