import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve } from 'node:path'

const output = resolve(process.cwd(), '.output/public')
const registry = JSON.parse(readFileSync(resolve(process.cwd(), '../registry/index.json'), 'utf8'))
const base = (process.env.NUXT_APP_BASE_URL ?? '/').replace(/\/?$/, '/')
const failures = []

const expected = ['index.html', 'plugins/index.html', 'robots.txt', 'sitemap.xml', ...registry.plugins.map(plugin => `plugins/${plugin.name}/index.html`)]
for (const file of expected) {
  const target = resolve(output, file)
  if (!existsSync(target) || !statSync(target).isFile()) failures.push(`missing prerendered file: ${file}`)
}

function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name)
    return entry.isDirectory() ? files(path) : [path]
  })
}

function targetExists(pathname) {
  let relative = decodeURIComponent(pathname.slice(base.length)).replace(/^\//, '')
  if (!relative || relative.endsWith('/')) relative += 'index.html'
  const direct = resolve(output, relative)
  return existsSync(direct) || existsSync(`${direct}.html`) || existsSync(resolve(direct, 'index.html'))
}

for (const file of files(output).filter(path => path.endsWith('.html'))) {
  const html = readFileSync(file, 'utf8')
  for (const match of html.matchAll(/(?:href|src)="([^"]+)"/g)) {
    const value = match[1]
    if (!value || value.startsWith('#') || /^(?:https?:|mailto:|data:)/.test(value)) continue
    const pathname = value.split(/[?#]/, 1)[0]
    if (!pathname.startsWith(base)) {
      failures.push(`${file.slice(output.length + 1)}: internal URL escapes Pages base: ${value}`)
    } else if (!targetExists(pathname)) {
      failures.push(`${file.slice(output.length + 1)}: broken internal URL: ${value}`)
    }
  }
}

if (failures.length) {
  throw new Error(`generated-site checks failed:\n${failures.join('\n')}`)
}
console.log(`generated-site checks passed (${expected.length} required routes, base ${base})`)
