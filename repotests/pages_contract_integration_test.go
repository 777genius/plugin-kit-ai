package pluginkitairepo_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPagesSite_CombinesLandingRootAndDocsSubpath(t *testing.T) {
	root := RepoRoot(t)

	workflowBody, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "docs-pages.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBody)
	mustContain(t, workflow, "name: Pages")
	mustContain(t, workflow, "working-directory: landing")
	mustContain(t, workflow, "pnpm generate")
	mustContain(t, workflow, "NUXT_APP_BASE_URL: /universal-agent-plugins/")
	mustContain(t, workflow, "DOCS_BASE_PATH: /universal-agent-plugins/docs/")
	mustContain(t, workflow, "go run ./cmd/agentplugins-registry-mirror")
	mustContain(t, workflow, "MIRROR_METADATA.json")
	mustContain(t, workflow, "777genius/universal-agent-plugins-registry")
	mustContain(t, workflow, "pnpm run build:pages")
	mustContain(t, workflow, "path: .pages-dist")

	packageBody, err := os.ReadFile(filepath.Join(root, "landing", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := string(packageBody)
	mustContain(t, pkg, `"build:pages": "node ./scripts/build-pages-artifact.mjs"`)

	scriptBody, err := os.ReadFile(filepath.Join(root, "landing", "scripts", "build-pages-artifact.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBody)
	mustContain(t, script, `const landingRoot = path.resolve(scriptDir, "..");`)
	mustContain(t, script, `const repoRoot = path.resolve(landingRoot, "..");`)
	mustContain(t, script, `const docsTarget = path.join(pagesDist, "docs");`)
	mustContain(t, script, `await fs.cp(landingDist, pagesDist, { recursive: true });`)
	mustContain(t, script, `await fs.cp(docsDist, docsTarget, { recursive: true });`)

	nodeRuntimeExtractorBody, err := os.ReadFile(filepath.Join(root, "website", "tools", "extractors", "node-runtime.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	nodeRuntimeExtractor := string(nodeRuntimeExtractorBody)
	mustContain(t, nodeRuntimeExtractor, `--tsconfig`)
	mustContain(t, nodeRuntimeExtractor, `../npm/plugin-kit-ai-runtime/tsconfig.docs.json`)

	nodeRuntimeTsconfigBody, err := os.ReadFile(filepath.Join(root, "npm", "plugin-kit-ai-runtime", "tsconfig.docs.json"))
	if err != nil {
		t.Fatal(err)
	}
	nodeRuntimeTsconfig := string(nodeRuntimeTsconfigBody)
	mustContain(t, nodeRuntimeTsconfig, `"ignoreDeprecations": "6.0"`)
	mustContain(t, nodeRuntimeTsconfig, `"include": ["index.d.ts"]`)

	siteBody, err := os.ReadFile(filepath.Join(root, "website", "tools", "config", "site.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	site := string(siteBody)
	mustContain(t, site, `export const docsBasePath = process.env.DOCS_BASE_PATH || "/plugin-kit-ai/docs/";`)

	i18nBody, err := os.ReadFile(filepath.Join(root, "landing", "data", "i18n.ts"))
	if err != nil {
		t.Fatal(err)
	}
	i18n := string(i18nBody)
	mustContain(t, i18n, `'/plugins'`)
	mustContain(t, i18n, `const pluginDetailPages =`)
	mustContain(t, i18n, "`/plugins/${plugin.slug ?? plugin.id}`")

	pluginDetailPageBody, err := os.ReadFile(filepath.Join(root, "landing", "pages", "plugins", "[slug].vue"))
	if err != nil {
		t.Fatal(err)
	}
	pluginDetailPage := string(pluginDetailPageBody)
	mustContain(t, pluginDetailPage, `sourceUrl(plugin)`)
	mustContain(t, pluginDetailPage, `aria-label="Back to plugin directory"`)

	docsConfigBody, err := os.ReadFile(filepath.Join(root, "website", ".vitepress", "config", "shared.ts"))
	if err != nil {
		t.Fatal(err)
	}
	docsConfig := string(docsConfigBody)
	mustContain(t, docsConfig, `logo: "/icon.svg"`)
	mustNotContain(t, docsConfig, "logo: `${docsBasePath}")

	robotsBody, err := os.ReadFile(filepath.Join(root, "landing", "server", "routes", "robots.txt.ts"))
	if err != nil {
		t.Fatal(err)
	}
	robots := string(robotsBody)
	mustContain(t, robots, `https://777genius.github.io/universal-agent-plugins/docs/sitemap.xml`)
}
