export default defineEventHandler(() => {
  const config = useRuntimeConfig()
  return `User-agent: *\nAllow: /\nSitemap: ${config.public.siteUrl}/sitemap.xml\n`
})
