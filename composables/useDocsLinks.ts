import { computed } from "vue";
import type { LocaleCode } from "~/data/i18n";

const docsLocalePattern = /\/(en|ru)(?=\/|$)/;

const replaceDocsLocale = (url: string, locale: LocaleCode): string => {
  if (!url) {
    return url;
  }

  if (docsLocalePattern.test(url)) {
    return url.replace(docsLocalePattern, `/${locale}`);
  }

  return url;
};

export const useDocsLinks = () => {
  const { locale } = useI18n();
  const config = useRuntimeConfig();

  const currentLocale = computed<LocaleCode>(() =>
    locale.value === "ru" ? "ru" : "en"
  );

  const docsUrl = computed(() =>
    replaceDocsLocale(
      config.public.docsUrl || "https://777genius.github.io/plugin-kit-ai/en/",
      currentLocale.value
    )
  );

  const quickstartUrl = computed(() =>
    replaceDocsLocale(
      config.public.quickstartUrl ||
        "https://777genius.github.io/plugin-kit-ai/en/guide/quickstart.html",
      currentLocale.value
    )
  );

  return { docsUrl, quickstartUrl };
};
