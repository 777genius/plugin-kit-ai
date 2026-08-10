import type { RegistryIndex } from '../../types/registry'

const escapeXml = (value: string) => value
  .replaceAll('&', '&amp;')
  .replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;')
  .replaceAll('"', '&quot;')
  .replaceAll("'", '&apos;')

export default defineEventHandler((event) => {
  const config = useRuntimeConfig()
  const registry = config.public.registryIndex as unknown as RegistryIndex
  const base = String(config.public.siteUrl).replace(/\/$/, '')
  const paths = ['/', '/plugins', ...registry.plugins.map(plugin => `/plugins/${plugin.name}`)]
  const urls = paths.map(path => `<url><loc>${escapeXml(`${base}${path}`)}</loc></url>`).join('')
  setHeader(event, 'content-type', 'application/xml; charset=utf-8')
  return `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${urls}</urlset>`
})
