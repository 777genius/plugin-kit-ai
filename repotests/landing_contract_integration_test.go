package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
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

	ruContentBody, err := os.ReadFile(filepath.Join(root, "content", "ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	ruContent := string(ruContentBody)
	mustContain(t, ruContent, `https://777genius.github.io/plugin-kit-ai/ru/guide/quickstart.html`)
	mustNotContain(t, ruContent, `"testimonials"`)
	mustContain(t, ruContent, `"title": "Проверяемый установочный скрипт"`)
	mustContain(t, ruContent, `"status": "Публичная бета"`)
	mustNotContain(t, ruContent, `Public-beta обёртка`)

	headerBody, err := os.ReadFile(filepath.Join(root, "components", "layout", "AppHeader.vue"))
	if err != nil {
		t.Fatal(err)
	}
	header := string(headerBody)
	mustContain(t, header, `const sectionHref = (sectionId: string) =>`)
	mustContain(t, header, "isHomePage.value ? `#${sectionId}` : `${homePath.value}#${sectionId}`")
	mustContain(t, header, `rel="noopener noreferrer"`)

	logoBody, err := os.ReadFile(filepath.Join(root, "components", "common", "AppLogo.vue"))
	if err != nil {
		t.Fatal(err)
	}
	logo := string(logoBody)
	mustContain(t, logo, `const localePath = useLocalePath();`)
	mustContain(t, logo, `<NuxtLink :to="homePath" class="app-logo">`)

	cmd := exec.Command("rg", "-n", "claude_agent_teams_ui|claude-agent-teams", filepath.Join(root, "components"), filepath.Join(root, "content"), filepath.Join(root, "composables"), filepath.Join(root, "locales"), filepath.Join(root, "types"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("legacy brand string still present:\n%s", out)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("rg legacy brand scan failed: %v\n%s", err, out)
	}
}
