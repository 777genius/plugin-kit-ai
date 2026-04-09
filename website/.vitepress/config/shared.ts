import fs from "node:fs";
import path from "node:path";
import { defineConfig } from "vitepress";

const websiteRoot = path.resolve(__dirname, "..", "..");
const generatedRoot = path.join(websiteRoot, "generated", "registries");

function readJson<T>(fileName: string, fallback: T): T {
  const full = path.join(generatedRoot, fileName);
  if (!fs.existsSync(full)) {
    return fallback;
  }
  return JSON.parse(fs.readFileSync(full, "utf8")) as T;
}

export const docsBasePath = process.env.DOCS_BASE_PATH || "/plugin-kit-ai/docs/";
export const docsHostname = process.env.DOCS_HOSTNAME || "https://777genius.github.io";
const docsBaseUrl = new URL(docsBasePath, docsHostname).toString();
const socialImageUrl = new URL("og-docs.svg", docsBaseUrl).toString();

function escapeForRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export const sharedConfig = defineConfig({
  srcDir: ".site",
  publicDir: path.join(websiteRoot, "public"),
  outDir: "dist",
  cleanUrls: true,
  lastUpdated: true,
  base: docsBasePath,
  title: "plugin-kit-ai Docs",
  description: "Public documentation for plugin-kit-ai",
  head: [
    ["link", { rel: "icon", href: `${docsBasePath}icon.svg`, type: "image/svg+xml" }],
    ["link", { rel: "manifest", href: `${docsBasePath}site.webmanifest` }],
    ["meta", { name: "theme-color", content: "#f7f7f8" }],
    ["meta", { name: "color-scheme", content: "light dark" }],
    ["meta", { property: "og:type", content: "website" }],
    ["meta", { property: "og:site_name", content: "plugin-kit-ai Docs" }],
    ["meta", { property: "og:title", content: "plugin-kit-ai Docs" }],
    ["meta", { property: "og:description", content: "Public documentation for plugin-kit-ai." }],
    ["meta", { property: "og:url", content: docsBaseUrl }],
    ["meta", { property: "og:image", content: socialImageUrl }],
    ["meta", { name: "twitter:card", content: "summary" }],
    ["meta", { name: "twitter:title", content: "plugin-kit-ai Docs" }],
    ["meta", { name: "twitter:description", content: "Public documentation for plugin-kit-ai." }],
    ["meta", { name: "twitter:image", content: socialImageUrl }]
  ],
  sitemap: {
    hostname: docsBaseUrl,
    transformItems(items) {
      const gatewayUrl = docsBaseUrl;
      const normalize = (value: string) => (value.endsWith("/") ? value : `${value}/`);
      return items.filter((item) => {
        const url = normalize(item.url);
        return url !== "/" && url !== normalize(docsBasePath) && url !== normalize(gatewayUrl);
      });
    }
  },
  async buildEnd() {
    const sitemapPath = path.join(websiteRoot, "dist", "sitemap.xml");
    if (!fs.existsSync(sitemapPath)) {
      return;
    }
    const gatewayUrl = docsBaseUrl;
    const entryPattern = new RegExp(`<url><loc>${escapeForRegExp(gatewayUrl)}<\\/loc>[\\s\\S]*?<\\/url>`, "g");
    const current = fs.readFileSync(sitemapPath, "utf8");
    const next = current.replace(entryPattern, "");
    if (next !== current) {
      fs.writeFileSync(sitemapPath, next);
    }

    const robotsPath = path.join(websiteRoot, "dist", "robots.txt");
    const robotsBody = [`User-agent: *`, `Allow: /`, `Sitemap: ${new URL("sitemap.xml", docsBaseUrl).toString()}`, ``].join(
      "\n"
    );
    fs.writeFileSync(robotsPath, robotsBody);
  },
  locales: {
    root: { label: "Language Gateway", lang: "en-US" },
    en: { label: "English", lang: "en-US" },
    ru: { label: "Русский", lang: "ru-RU" },
    es: { label: "Español", lang: "es-ES" },
    fr: { label: "Français", lang: "fr-FR" },
    zh: { label: "简体中文", lang: "zh-CN" }
  },
  rewrites: readJson<Record<string, string>>("redirects.json", {}),
  themeConfig: {
    siteTitle: "plugin-kit-ai Docs",
    search: {
      provider: "local"
    },
    socialLinks: [{ icon: "github", link: "https://github.com/777genius/plugin-kit-ai" }]
  }
});
