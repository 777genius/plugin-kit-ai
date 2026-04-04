package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLandingSurface_LocalesLinksAndBrandingStayAligned(t *testing.T) {
	root := RepoRoot(t)

	i18nBody, err := os.ReadFile(filepath.Join(root, "data", "i18n.ts"))
	if err != nil {
		t.Fatal(err)
	}
	i18n := string(i18nBody)
	mustContain(t, i18n, `export type LocaleCode = "en" | "ru";`)
	mustContain(t, i18n, `{ code: "en"`)
	mustContain(t, i18n, `{ code: "ru"`)
	mustNotContain(t, i18n, `{ code: "de"`)
	mustNotContain(t, i18n, `{ code: "fr"`)

	docsLinksBody, err := os.ReadFile(filepath.Join(root, "composables", "useDocsLinks.ts"))
	if err != nil {
		t.Fatal(err)
	}
	docsLinks := string(docsLinksBody)
	mustContain(t, docsLinks, `const docsLocalePattern = /\/(en|ru)(?=\/|$)/;`)
	mustContain(t, docsLinks, `locale.value === "ru" ? "ru" : "en"`)
	mustContain(t, docsLinks, `supportBoundaryUrl`)
	mustContain(t, docsLinks, `https://777genius.github.io/plugin-kit-ai/docs/en/`)

	releaseComposableBody, err := os.ReadFile(filepath.Join(root, "composables", "useReleaseDownloads.ts"))
	if err != nil {
		t.Fatal(err)
	}
	releaseComposable := string(releaseComposableBody)
	mustContain(t, releaseComposable, `"/api/releases/latest"`)
	mustContain(t, releaseComposable, `server: true`)
	mustContain(t, releaseComposable, `lazy: false`)
	mustContain(t, releaseComposable, `plugin-kit-ai_release_meta`)

	releaseRouteBody, err := os.ReadFile(filepath.Join(root, "server", "api", "releases", "latest.get.ts"))
	if err != nil {
		t.Fatal(err)
	}
	releaseRoute := string(releaseRouteBody)
	mustContain(t, releaseRoute, `https://api.github.com/repos/${githubRepo}/releases/latest`)
	mustContain(t, releaseRoute, `cache-control`)
	mustContain(t, releaseRoute, `RELEASE_CACHE_TTL`)

	sectionsBody, err := os.ReadFile(filepath.Join(root, "data", "sections.ts"))
	if err != nil {
		t.Fatal(err)
	}
	sections := string(sectionsBody)
	mustNotContain(t, sections, `"pricing"`)

	seoBody, err := os.ReadFile(filepath.Join(root, "composables", "usePageSeo.ts"))
	if err != nil {
		t.Fatal(err)
	}
	seo := string(seoBody)
	mustContain(t, seo, `https://777genius.github.io/plugin-kit-ai`)
	mustNotContain(t, seo, `hookplex.dev`)
	mustNotContain(t, seo, `priceCurrency`)
	mustNotContain(t, seo, `offers:`)

	nuxtConfigBody, err := os.ReadFile(filepath.Join(root, "nuxt.config.ts"))
	if err != nil {
		t.Fatal(err)
	}
	nuxtConfig := string(nuxtConfigBody)
	mustContain(t, nuxtConfig, `https://777genius.github.io/plugin-kit-ai/docs/en/`)
	mustContain(t, nuxtConfig, `https://777genius.github.io/plugin-kit-ai/docs/sitemap.xml`)

	ruContentBody, err := os.ReadFile(filepath.Join(root, "content", "ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	ruContent := string(ruContentBody)
	mustContain(t, ruContent, `https://777genius.github.io/plugin-kit-ai/docs/ru/guide/quickstart.html`)
	mustNotContain(t, ruContent, `"testimonials"`)
	mustContain(t, ruContent, `"title": "Проверяемый установочный скрипт"`)
	mustContain(t, ruContent, `"status": "Публичная бета"`)
	mustContain(t, ruContent, `"pluginKitAi": { "status": "yes"`)
	mustNotContain(t, ruContent, `Public-beta обёртка`)
	mustNotContain(t, ruContent, `"pricing"`)
	mustNotContain(t, ruContent, `"hookplex":`)

	enLocaleBody, err := os.ReadFile(filepath.Join(root, "locales", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	enLocale := string(enLocaleBody)
	mustContain(t, enLocale, `"copy": "Copy"`)
	mustContain(t, enLocale, `"copied": "Copied"`)
	mustContain(t, enLocale, `"comparison": "Why it works"`)
	mustContain(t, enLocale, `"pluginKitAi": "plugin-kit-ai"`)
	mustContain(t, enLocale, `"render": "generate outputs"`)
	mustNotContain(t, enLocale, `"pricing"`)
	mustNotContain(t, enLocale, `Hookplex`)
	mustNotContain(t, enLocale, `"hookplex":`)
	mustNotContain(t, enLocale, `"render": "render"`)

	ruLocaleBody, err := os.ReadFile(filepath.Join(root, "locales", "ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	ruLocale := string(ruLocaleBody)
	mustContain(t, ruLocale, `"copy": "Копировать"`)
	mustContain(t, ruLocale, `"copied": "Скопировано"`)
	mustContain(t, ruLocale, `"comparison": "Почему это работает"`)
	mustContain(t, ruLocale, `"pluginKitAi": "plugin-kit-ai"`)
	mustContain(t, ruLocale, `"render": "собрать варианты"`)
	mustNotContain(t, ruLocale, `"pricing"`)
	mustNotContain(t, ruLocale, `Hookplex`)
	mustNotContain(t, ruLocale, `"hookplex":`)
	mustNotContain(t, ruLocale, `"render": "render"`)

	headerBody, err := os.ReadFile(filepath.Join(root, "components", "layout", "AppHeader.vue"))
	if err != nil {
		t.Fatal(err)
	}
	header := string(headerBody)
	mustContain(t, header, `const router = useRouter();`)
	mustContain(t, header, `const homeHref = computed(() => router.resolve(homePath.value).href);`)
	mustContain(t, header, `const sectionHref = (sectionId: string) =>`)
	mustContain(t, header, "isHomePage.value ? `#${sectionId}` : `${homeHref.value}#${sectionId}`")
	mustContain(t, header, `rel="noopener noreferrer"`)
	mustNotContain(t, header, `nav.pricing`)

	downloadBody, err := os.ReadFile(filepath.Join(root, "components", "sections", "DownloadSection.vue"))
	if err != nil {
		t.Fatal(err)
	}
	download := string(downloadBody)
	mustContain(t, download, `navigator.clipboard?.writeText`)
	mustContain(t, download, `download.copy`)
	mustContain(t, download, `download.copied`)

	robotsBody, err := os.ReadFile(filepath.Join(root, "server", "routes", "robots.txt.ts"))
	if err != nil {
		t.Fatal(err)
	}
	robots := string(robotsBody)
	mustContain(t, robots, `https://777genius.github.io/plugin-kit-ai/docs/sitemap.xml`)

	logoBody, err := os.ReadFile(filepath.Join(root, "components", "common", "AppLogo.vue"))
	if err != nil {
		t.Fatal(err)
	}
	logo := string(logoBody)
	mustContain(t, logo, `const localePath = useLocalePath();`)
	mustContain(t, logo, `<NuxtLink :to="homePath" class="app-logo">`)
	mustContain(t, logo, `plugin-kit-ai`)
	mustNotContain(t, logo, `Hookplex`)

	heroBody, err := os.ReadFile(filepath.Join(root, "components", "sections", "HeroSection.vue"))
	if err != nil {
		t.Fatal(err)
	}
	hero := string(heroBody)
	mustContain(t, hero, `<span class="hero-section__logo">P</span>`)
	mustNotContain(t, hero, `<span class="hero-section__logo">H</span>`)

	indexBody, err := os.ReadFile(filepath.Join(root, "pages", "index.vue"))
	if err != nil {
		t.Fatal(err)
	}
	indexPage := string(indexBody)
	mustNotContain(t, indexPage, `LazyPricingSection`)

	matches, err := scanLegacyBranding(root)
	if err != nil {
		t.Fatalf("legacy brand scan failed: %v", err)
	}
	if len(matches) > 0 {
		t.Fatalf("legacy brand string still present:\n%s", strings.Join(matches, "\n"))
	}
}

func scanLegacyBranding(root string) ([]string, error) {
	searchRoots := []string{
		filepath.Join(root, "components"),
		filepath.Join(root, "content"),
		filepath.Join(root, "composables"),
		filepath.Join(root, "locales"),
		filepath.Join(root, "types"),
	}
	if _, err := exec.LookPath("rg"); err == nil {
		args := append([]string{"-n", "claude_agent_teams_ui|claude-agent-teams"}, searchRoots...)
		out, scanErr := exec.Command("rg", args...).CombinedOutput()
		if scanErr == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) == 1 && lines[0] == "" {
				return nil, nil
			}
			return lines, nil
		}
		if exitErr, ok := scanErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, scanErr
	}

	patterns := []string{"claude_agent_teams_ui", "claude-agent-teams"}
	var matches []string
	for _, base := range searchRoots {
		walkErr := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(body)
			for _, pattern := range patterns {
				if strings.Contains(text, pattern) {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						rel = path
					}
					matches = append(matches, filepath.ToSlash(rel)+": "+pattern)
					break
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return matches, nil
}
