import { resolve } from 'node:path'
import { loadRegistryIndex } from './build/load-registry'

const defaultRegistryPath = resolve(process.cwd(), '../registry/index.json')
const registryPath = process.env.UAP_REGISTRY_PATH
  ? resolve(process.cwd(), process.env.UAP_REGISTRY_PATH)
  : defaultRegistryPath
const registryIndex = loadRegistryIndex(registryPath)
const builtInCount = registryIndex.plugins.filter(plugin => plugin.built_in).length

if (!process.env.UAP_REGISTRY_PATH && builtInCount !== 26) {
  throw new Error(`Production registry must contain exactly 26 built-in plugins; found ${builtInCount}`)
}

const siteUrl = (process.env.NUXT_PUBLIC_SITE_URL
  ?? 'https://777genius.github.io/universal-agent-plugins').replace(/\/$/, '')
const baseURL = process.env.NUXT_APP_BASE_URL ?? '/'
const repositoryUrl = 'https://github.com/777genius/universal-agent-plugins'

export default defineNuxtConfig({
  compatibilityDate: '2026-08-10',
  ssr: true,
  devtools: { enabled: false },
  modules: ['@nuxt/eslint'],
  css: ['~/assets/css/main.css'],
  app: {
    baseURL,
    head: {
      htmlAttrs: { lang: 'en' },
      titleTemplate: '%s · Universal Agent Plugins',
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: `${baseURL}logo.svg` },
      ],
      script: [{
        innerHTML: "try{document.documentElement.dataset.theme=localStorage.getItem('uap-theme')||((matchMedia('(prefers-color-scheme:light)').matches)?'light':'dark')}catch(e){}",
      }],
    },
  },
  runtimeConfig: {
    public: {
      registryIndex,
      siteUrl,
      baseURL,
      repositoryUrl,
    },
  },
  nitro: {
    compressPublicAssets: true,
    prerender: {
      crawlLinks: false,
      routes: [
        '/',
        '/plugins',
        '/robots.txt',
        '/sitemap.xml',
        ...registryIndex.plugins.map(plugin => `/plugins/${plugin.name}`),
      ],
    },
  },
  routeRules: {
    '/**': { prerender: true },
    '/_nuxt/**': { headers: { 'cache-control': 'public, max-age=31536000, immutable' } },
  },
  typescript: {
    strict: true,
    typeCheck: true,
  },
})
