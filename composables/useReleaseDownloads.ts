type DownloadOs = "macos" | "windows" | "linux"
type DownloadArch = "arm64" | "x64" | "universal"

type ReleaseAsset = {
  name: string
  browser_download_url: string
}

type GitHubRelease = {
  tag_name: string
  published_at: string
  assets: ReleaseAsset[]
}

type Variant = {
  url: string | null
  platformKey: string | null
  version: string | null
}

type DownloadsApiResponse = {
  ok: boolean
  source: "github-releases"
  fetchedAt: string
  version: string | null
  pubDate: string | null
  variants: {
    macos: { arm64: Variant; x64: Variant; universal: Variant }
    windows: { x64: Variant }
    linux: { appimage: Variant; deb: Variant }
  }
}

type ResolveResult = { url: string; version: string | null } | null

const CACHE_KEY = "hookplex_release_meta"
const CACHE_TTL = 10 * 60 * 1000
const emptyVariant: Variant = { url: null, platformKey: null, version: null }

function isClient(): boolean {
  return typeof window !== "undefined"
}

function readCache(): DownloadsApiResponse | null {
  if (!isClient()) {
    return null
  }

  try {
    const raw = window.sessionStorage.getItem(CACHE_KEY)
    if (!raw) {
      return null
    }

    const parsed = JSON.parse(raw) as { ts: number; data: DownloadsApiResponse }
    if (Date.now() - parsed.ts > CACHE_TTL) {
      return null
    }

    return parsed.data
  } catch {
    return null
  }
}

function writeCache(data: DownloadsApiResponse): void {
  if (!isClient()) {
    return
  }

  try {
    window.sessionStorage.setItem(
      CACHE_KEY,
      JSON.stringify({ ts: Date.now(), data })
    )
  } catch {
    // Ignore unavailable session storage.
  }
}

function findAsset(assets: ReleaseAsset[], pattern: RegExp): ReleaseAsset | null {
  return assets.find((asset) => pattern.test(asset.name)) || null
}

function toVariant(
  asset: ReleaseAsset | null,
  version: string | null
): Variant {
  if (!asset) {
    return { ...emptyVariant }
  }

  return {
    url: asset.browser_download_url,
    platformKey: asset.name,
    version
  }
}

function parseGitHubRelease(release: GitHubRelease): DownloadsApiResponse {
  const version = release.tag_name?.replace(/^v/, "") || null
  const assets = release.assets || []

  return {
    ok: true,
    source: "github-releases",
    fetchedAt: new Date().toISOString(),
    version,
    pubDate: release.published_at || null,
    variants: {
      macos: {
        arm64: toVariant(findAsset(assets, /darwin_arm64\.tar\.gz$/i), version),
        x64: toVariant(findAsset(assets, /darwin_amd64\.tar\.gz$/i), version),
        universal: { ...emptyVariant }
      },
      windows: {
        x64: toVariant(findAsset(assets, /windows_amd64\.zip$/i), version)
      },
      linux: {
        appimage: toVariant(findAsset(assets, /\.AppImage$/i), version),
        deb: toVariant(findAsset(assets, /\.deb$/i), version)
      }
    }
  }
}

function emptyResponse(): DownloadsApiResponse {
  return {
    ok: false,
    source: "github-releases",
    fetchedAt: new Date().toISOString(),
    version: null,
    pubDate: null,
    variants: {
      macos: {
        arm64: { ...emptyVariant },
        x64: { ...emptyVariant },
        universal: { ...emptyVariant }
      },
      windows: {
        x64: { ...emptyVariant }
      },
      linux: {
        appimage: { ...emptyVariant },
        deb: { ...emptyVariant }
      }
    }
  }
}

export const useReleaseDownloads = () => {
  const config = useRuntimeConfig()
  const githubRepo = config.public.githubRepo || "777genius/plugin-kit-ai"
  const fallbackUrl =
    config.public.githubReleasesUrl || `https://github.com/${githubRepo}/releases`

  const { data, pending, error } = useAsyncData<DownloadsApiResponse>(
    "hookplex-releases",
    async () => {
      const cached = readCache()
      if (cached) {
        return cached
      }

      try {
        const release = await $fetch<GitHubRelease>(
          `https://api.github.com/repos/${githubRepo}/releases/latest`,
          {
            headers: {
              Accept: "application/vnd.github+json"
            }
          }
        )

        const parsed = parseGitHubRelease(release)
        writeCache(parsed)
        return parsed
      } catch {
        return emptyResponse()
      }
    },
    {
      server: false,
      lazy: true,
      default: () => emptyResponse()
    }
  )

  const resolve = (
    os: DownloadOs,
    arch: DownloadArch | "unknown"
  ): ResolveResult => {
    const api = data.value
    if (!api.ok) {
      return null
    }

    if (os === "windows") {
      const variant = api.variants.windows.x64
      return variant.url
        ? { url: variant.url, version: variant.version || api.version }
        : null
    }

    if (os === "linux") {
      const variant = api.variants.linux.appimage.url
        ? api.variants.linux.appimage
        : api.variants.linux.deb
      return variant.url
        ? { url: variant.url, version: variant.version || api.version }
        : null
    }

    if (os === "macos") {
      const universal = api.variants.macos.universal
      if (universal.url) {
        return { url: universal.url, version: universal.version || api.version }
      }

      const byArch =
        arch === "arm64" ? api.variants.macos.arm64 : api.variants.macos.x64
      if (byArch.url) {
        return { url: byArch.url, version: byArch.version || api.version }
      }

      const any = api.variants.macos.arm64.url
        ? api.variants.macos.arm64
        : api.variants.macos.x64
      return any.url ? { url: any.url, version: any.version || api.version } : null
    }

    return null
  }

  const resolveUrlOrFallback = (
    os: DownloadOs,
    arch: DownloadArch | "unknown"
  ): string => resolve(os, arch)?.url || fallbackUrl

  return { data, pending, error, fallbackUrl, resolve, resolveUrlOrFallback }
}
