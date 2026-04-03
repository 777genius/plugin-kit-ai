import vuetify from "vite-plugin-vuetify";
import { generateI18nRoutes, supportedLocales } from "./data/i18n";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
declare const process: any;

const siteUrl = process.env.NUXT_PUBLIC_SITE_URL || "https://777genius.github.io/plugin-kit-ai";
const githubRepo = process.env.NUXT_PUBLIC_GITHUB_REPO || "777genius/plugin-kit-ai";
const githubReleasesUrl = `https://github.com/${githubRepo}/releases`;
const docsUrl = process.env.NUXT_PUBLIC_DOCS_URL || "https://777genius.github.io/plugin-kit-ai/docs/en/";
const quickstartUrl =
  process.env.NUXT_PUBLIC_QUICKSTART_URL || "https://777genius.github.io/plugin-kit-ai/docs/en/guide/quickstart.html";
const docsSitemapUrl =
  process.env.NUXT_PUBLIC_DOCS_SITEMAP_URL || "https://777genius.github.io/plugin-kit-ai/docs/sitemap.xml";
const baseURL = process.env.NUXT_APP_BASE_URL || "/";

export default defineNuxtConfig({
  compatibilityDate: "2026-01-19",
  ssr: true,
  app: {
    baseURL,
    head: {
      link: [
        { rel: "icon", type: "image/svg+xml", href: `${baseURL}icon.svg` },
        { rel: "dns-prefetch", href: "https://api.github.com" },
        { rel: "preconnect", href: "https://fonts.googleapis.com" },
        { rel: "preconnect", href: "https://fonts.gstatic.com", crossorigin: "" },
        { rel: "preload", href: "https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;800&family=JetBrains+Mono:wght@400;600&display=swap", as: "style" },
        { rel: "stylesheet", href: "https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;800&family=JetBrains+Mono:wght@400;600&display=swap" }
      ]
    }
  },
  modules: [
    "@pinia/nuxt",
    "@nuxtjs/i18n",
    "@vueuse/nuxt",
    "nuxt-icon",
    "@nuxt/eslint"
  ],
  css: ["~/assets/styles/main.scss"],
  components: [
    {
      path: "~/components",
      pathPrefix: false
    }
  ],
  build: {
    transpile: ["vuetify"]
  },
  vue: {
    compilerOptions: {
      isCustomElement: (tag: string) => tag.startsWith("swiper-")
    }
  },
  vite: {
    plugins: [vuetify({ autoImport: true })]
  },
  nitro: {
    compressPublicAssets: true,
    prerender: {
      routes: [
        ...generateI18nRoutes(),
        "/sitemap.xml",
        "/robots.txt"
      ]
    }
  },
  routeRules: {
    "/_nuxt/**": {
      headers: { "Cache-Control": "public, max-age=31536000, immutable" }
    }
  },
  i18n: {
    restructureDir: false,
    locales: supportedLocales,
    defaultLocale: "en",
    strategy: "prefix_except_default",
    lazy: true,
    langDir: "locales",
    bundle: {
      optimizeTranslationDirective: false
    },
    detectBrowserLanguage: {
      useCookie: true,
      cookieKey: "i18n_redirected",
      redirectOn: "root",
      alwaysRedirect: false,
      fallbackLocale: "en"
    }
  },
  // @ts-expect-error - field provided by nuxt modules
  site: {
    url: siteUrl,
    name: "plugin-kit-ai"
  },
  runtimeConfig: {
    github: {
      token: process.env.GITHUB_TOKEN
    },
    public: {
      siteUrl,
      githubRepo,
      githubReleasesUrl,
      docsUrl,
      quickstartUrl,
      docsSitemapUrl
    }
  }
});
