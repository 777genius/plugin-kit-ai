export default defineEventHandler((event) => {
  const config = useRuntimeConfig();
  const siteUrl = (config.public.siteUrl as string) || "https://777genius.github.io/plugin-kit-ai";
  const docsSitemapUrl =
    (config.public.docsSitemapUrl as string) || "https://777genius.github.io/plugin-kit-ai/docs/sitemap.xml";

  setHeader(event, "content-type", "text/plain; charset=utf-8");

  return `User-agent: *
Allow: /
Sitemap: ${siteUrl}/sitemap.xml
Sitemap: ${docsSitemapUrl}
`;
});
