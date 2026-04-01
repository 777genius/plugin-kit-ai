import DefaultTheme from "vitepress/theme";
import type { Theme } from "vitepress";
import { h } from "vue";
import LanguageGateway from "./components/LanguageGateway.vue";
import LocalePreferenceSync from "./components/LocalePreferenceSync.vue";
import DocMetaCard from "./components/DocMetaCard.vue";
import MermaidDiagram from "./components/MermaidDiagram.vue";
import NotFoundLinks from "./components/NotFoundLinks.vue";
import "./styles/custom.css";

const theme: Theme = {
  ...DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      "layout-top": () => h(LocalePreferenceSync)
    }),
  enhanceApp({ app }) {
    app.component("LanguageGateway", LanguageGateway);
    app.component("LocalePreferenceSync", LocalePreferenceSync);
    app.component("DocMetaCard", DocMetaCard);
    app.component("MermaidDiagram", MermaidDiagram);
    app.component("NotFoundLinks", NotFoundLinks);
  }
};

export default theme;
