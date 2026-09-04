import vuetify from 'vite-plugin-vuetify';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { supportedLocales } from './data/i18n';
import { loadRegistryIndex } from './build/load-registry';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
declare const process: any;

const siteUrl =
  process.env.NUXT_PUBLIC_SITE_URL || 'https://777genius.github.io/universal-agent-plugins';
const githubRepo = process.env.NUXT_PUBLIC_GITHUB_REPO || '777genius/universal-agent-plugins';
const productName = process.env.NUXT_PUBLIC_PRODUCT_NAME || 'Universal Agent Plugins';
const githubReleasesUrl = `https://github.com/${githubRepo}/releases`;
const docsUrl =
  process.env.NUXT_PUBLIC_DOCS_URL ||
  'https://777genius.github.io/universal-agent-plugins/docs/en/';
const quickstartUrl =
  process.env.NUXT_PUBLIC_QUICKSTART_URL ||
  'https://777genius.github.io/universal-agent-plugins/docs/en/guide/quickstart.html';
const docsSitemapUrl =
  process.env.NUXT_PUBLIC_DOCS_SITEMAP_URL ||
  'https://777genius.github.io/universal-agent-plugins/docs/sitemap.xml';
const baseURL = process.env.NUXT_APP_BASE_URL || '/';
const registryRepositoryUrl =
  process.env.NUXT_PUBLIC_REGISTRY_REPOSITORY_URL ||
  'https://github.com/777genius/universal-agent-plugins-registry';
const discoveryKeyID = process.env.NUXT_PUBLIC_DISCOVERY_KEY_ID || 'uap-discovery-2026-01';
const discoveryPublicKey =
  process.env.NUXT_PUBLIC_DISCOVERY_PUBLIC_KEY || 'IxWvGuscXR9crlCrGyBQZNqroYNVPbBA1B3pnjSffhc=';

function resolveRegistrySnapshot(): string {
  const explicit = process.env.UAP_SIGNED_SNAPSHOT_PATH;
  if (explicit) return resolve(process.cwd(), explicit);

  const pointerPath = resolve(process.cwd(), 'public/registry/schemas/1/latest.json');
  if (!existsSync(pointerPath)) {
    throw new Error(
      'Signed registry mirror is missing. Run agentplugins-registry-mirror before preparing or building the landing site.',
    );
  }
  const pointer = JSON.parse(readFileSync(pointerPath, 'utf8')) as { snapshot_path?: unknown };
  if (typeof pointer.snapshot_path !== 'string' || !pointer.snapshot_path) {
    throw new Error(`Invalid signed registry pointer at ${pointerPath}`);
  }
  return resolve(dirname(pointerPath), pointer.snapshot_path);
}

const registryIndex = loadRegistryIndex(resolveRegistrySnapshot(), 'published_snapshot');

export default defineNuxtConfig({
  compatibilityDate: '2026-01-19',
  ssr: true,
  experimental: {
    // Work around the current Nuxt dev-time #app-manifest regression.
    appManifest: false,
  },
  app: {
    baseURL,
    head: {
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: `${baseURL}icon.svg` },
        { rel: 'dns-prefetch', href: 'https://api.github.com' },
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'preload',
          href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;800&family=JetBrains+Mono:wght@400;600&display=swap',
          as: 'style',
        },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;800&family=JetBrains+Mono:wght@400;600&display=swap',
        },
      ],
    },
  },
  modules: ['@pinia/nuxt', '@nuxtjs/i18n', '@vueuse/nuxt', 'nuxt-icon', '@nuxt/eslint'],
  css: ['~/assets/styles/main.scss', '~/assets/styles/registry.scss'],
  components: [
    {
      path: '~/components',
      pathPrefix: false,
    },
  ],
  build: {
    transpile: ['vuetify'],
  },
  vue: {
    compilerOptions: {
      isCustomElement: (tag: string) => tag.startsWith('swiper-'),
    },
  },
  vite: {
    plugins: [vuetify({ autoImport: true })],
  },
  nitro: {
    compressPublicAssets: true,
    prerender: {
      crawlLinks: false,
      routes: [
        '/',
        '/create-plugin',
        '/plugins',
        '/plugins/community',
        ...registryIndex.plugins.map((plugin) => `/plugins/${plugin.name}`),
        '/api/releases/latest',
        '/sitemap.xml',
        '/robots.txt',
      ],
    },
  },
  routeRules: {
    '/_nuxt/**': {
      headers: { 'Cache-Control': 'public, max-age=31536000, immutable' },
    },
  },
  i18n: {
    restructureDir: false,
    locales: [...supportedLocales],
    defaultLocale: 'en',
    strategy: 'prefix_except_default',
    lazy: true,
    langDir: 'locales',
    bundle: {
      optimizeTranslationDirective: false,
    },
    detectBrowserLanguage: {
      useCookie: true,
      cookieKey: 'i18n_redirected',
      redirectOn: 'root',
      alwaysRedirect: false,
      fallbackLocale: 'en',
    },
  },
  // @ts-expect-error - field provided by nuxt modules
  site: {
    url: siteUrl,
    name: productName,
  },
  runtimeConfig: {
    github: {
      token: process.env.GITHUB_TOKEN,
    },
    public: {
      siteUrl,
      githubRepo,
      productName,
      githubReleasesUrl,
      docsUrl,
      quickstartUrl,
      docsSitemapUrl,
      baseURL,
      repositoryUrl: registryRepositoryUrl,
      registryIndex,
      discoveryKeyID,
      discoveryPublicKey,
    },
  },
});
