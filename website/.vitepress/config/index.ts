import { defineConfig } from "vitepress";
import { sharedConfig } from "./shared";
import { enLocaleConfig } from "./locales.en";
import { esLocaleConfig } from "./locales.es";
import { frLocaleConfig } from "./locales.fr";
import { ruLocaleConfig } from "./locales.ru";
import { zhLocaleConfig } from "./locales.zh";

export default defineConfig({
  ...sharedConfig,
  locales: {
    root: sharedConfig.locales?.root,
    en: enLocaleConfig,
    ru: ruLocaleConfig,
    es: esLocaleConfig,
    fr: frLocaleConfig,
    zh: zhLocaleConfig
  }
});
