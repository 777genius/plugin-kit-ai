import { computed } from "vue"
import { defaultLocale, supportedLocales } from "~/data/i18n"
import { getContent } from "~/data/content"
import type { LocaleCode } from "~/data/i18n"

type PageSeoImage = {
  url: string
  width?: number
  height?: number
  type?: string
  alt?: string
}

type PageSeoOptions = {
  type?: "website" | "article"
  robots?: string
  image?: PageSeoImage
}

export const usePageSeo = (
  titleKey: string,
  descriptionKey: string,
  options: PageSeoOptions = {}
) => {
  const { t, locale } = useI18n()
  const route = useRoute()
  const config = useRuntimeConfig()
  const switchLocale = useSwitchLocalePath()
  const { docsUrl } = useDocsLinks()
  const siteUrl = config.public.siteUrl || "https://777genius.github.io/plugin-kit-ai"
  const siteName = "plugin-kit-ai"
  const githubUrl = `https://github.com/${config.public.githubRepo}`

  const title = computed(() => t(titleKey))
  const description = computed(() => t(descriptionKey))
  const canonicalPath = computed(() => route.path)
  const canonicalUrl = computed(() => `${siteUrl}${canonicalPath.value}`)

  const resolvedImage = computed<PageSeoImage>(() => {
    if (options.image) {
      return options.image
    }

    return {
      url: "/og-image.svg",
      width: 1200,
      height: 630,
      type: "image/svg+xml",
      alt: "plugin-kit-ai - build a plugin once and ship it to many AI agents"
    }
  })

  const resolvedImageUrl = computed(() => {
    const imageUrl = resolvedImage.value.url
    return imageUrl.startsWith("http")
      ? imageUrl
      : new URL(imageUrl, siteUrl).toString()
  })

  useSeoMeta({
    title,
    description,
    ogTitle: title,
    ogDescription: description,
    ogType: options.type || "website",
    ogSiteName: siteName,
    ogUrl: canonicalUrl,
    ogImage: resolvedImageUrl,
    ogImageType: computed(() => resolvedImage.value.type) as never,
    ogImageWidth: computed(() =>
      resolvedImage.value.width ? String(resolvedImage.value.width) : undefined
    ),
    ogImageHeight: computed(() =>
      resolvedImage.value.height ? String(resolvedImage.value.height) : undefined
    ),
    ogImageAlt: computed(() => resolvedImage.value.alt),
    twitterCard: "summary_large_image",
    twitterTitle: title,
    twitterDescription: description,
    twitterImage: resolvedImageUrl,
    twitterImageAlt: computed(() => resolvedImage.value.alt),
    robots:
      options.robots ||
      "index, follow, max-snippet:-1, max-image-preview:large, max-video-preview:-1"
  })

  useHead(() => {
    const links: { rel: string; hreflang?: string; href: string }[] = supportedLocales.map(
      (item) => {
        const path = switchLocale(item.code) || canonicalPath.value
        return {
          rel: "alternate",
          hreflang: item.code,
          href: `${siteUrl}${path}`
        }
      }
    )

    const defaultPath = switchLocale(defaultLocale) || canonicalPath.value
    links.push({
      rel: "alternate",
      hreflang: "x-default",
      href: `${siteUrl}${defaultPath}`
    })
    links.push({ rel: "canonical", href: canonicalUrl.value })

    const jsonLd: Record<string, unknown>[] = [
      {
        "@context": "https://schema.org",
        "@type": "WebSite",
        name: siteName,
        url: siteUrl,
        inLanguage: supportedLocales.map((item) => item.code),
        description: description.value
      },
      {
        "@context": "https://schema.org",
        "@type": "Organization",
        name: siteName,
        url: siteUrl,
        logo: `${siteUrl}/icon.svg`,
        sameAs: [githubUrl]
      }
    ]

    const isDownload = canonicalPath.value.endsWith("/download")
    const isHome = canonicalPath.value === "/" || canonicalPath.value === "/ru"

    if (isHome || isDownload) {
      jsonLd.push({
        "@context": "https://schema.org",
        "@type": "SoftwareApplication",
        name: "plugin-kit-ai",
        applicationCategory: "DeveloperApplication",
        operatingSystem: "macOS, Linux, Windows",
        description: description.value,
        url: canonicalUrl.value,
        downloadUrl:
          config.public.githubReleasesUrl || `${githubUrl}/releases`,
        softwareHelp: docsUrl.value
      })
    }

    if (isHome) {
      const content = getContent(locale.value as LocaleCode)
      if (content.faq.length > 0) {
        jsonLd.push({
          "@context": "https://schema.org",
          "@type": "FAQPage",
          mainEntity: content.faq.map((item) => ({
            "@type": "Question",
            name: item.question,
            acceptedAnswer: {
              "@type": "Answer",
              text: item.answer.replace(/<[^>]*>/g, "")
            }
          }))
        })
      }
    }

    return {
      htmlAttrs: {
        lang: locale.value || "en"
      },
      link: links,
      meta: [
        { name: "author", content: "plugin-kit-ai" },
        { name: "application-name", content: siteName },
        { name: "apple-mobile-web-app-title", content: siteName },
        { name: "format-detection", content: "telephone=no" },
        { name: "theme-color", content: "#00f0ff" },
        {
          name: "keywords",
          content:
            "plugin-kit-ai, AI plugins, Claude plugins, Codex plugins, Gemini plugins, multi-agent plugins, plugin repo, validate strict"
        }
      ],
      script: jsonLd.map((item) => ({
        type: "application/ld+json",
        children: JSON.stringify(item)
      }))
    }
  })
}
