export default defineNuxtPlugin({
  name: "init-theme-locale",
  dependsOn: ["vuetify"],
  setup(nuxtApp) {
    const { initTheme } = useBrowserTheme();
    const { initLocale } = useLocation();

    // Run after hydration to avoid SSR/CSR mismatches.
    nuxtApp.hook("app:mounted", () => {
      initTheme();
      initLocale();
    });
  }
});
