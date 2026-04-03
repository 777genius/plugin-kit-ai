export type LocaleCode = "en" | "ru";

export const supportedLocales = [
  { code: "en", iso: "en-US", name: "English", flag: "\u{1F1FA}\u{1F1F8}", file: "en.json" },
  { code: "ru", iso: "ru-RU", name: "Русский", flag: "\u{1F1F7}\u{1F1FA}", file: "ru.json" }
] as const;

export const defaultLocale: LocaleCode = "en";

export const pages = [
  "/",
  "/download"
] as const;

/** Pages for sitemap */
export const sitemapPages = [
  "/",
  "/download"
] as const;

/** Generates i18n routes for a given list of pages */
const buildI18nRoutes = (source: readonly string[]): string[] => {
  const routes: string[] = [];
  for (const page of source) {
    routes.push(page);
    for (const locale of supportedLocales) {
      if (locale.code === defaultLocale) continue;
      routes.push(page === "/" ? `/${locale.code}` : `/${locale.code}${page}`);
    }
  }
  return routes;
};

/** All i18n routes (for prerender) */
export const generateI18nRoutes = (): string[] => buildI18nRoutes(pages);

/** i18n routes for sitemap only */
export const generateSitemapRoutes = (): string[] => buildI18nRoutes(sitemapPages);
