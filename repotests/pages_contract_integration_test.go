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
	mustContain(t, workflow, "pnpm generate")
	mustContain(t, workflow, "NUXT_APP_BASE_URL: /plugin-kit-ai/")
	mustContain(t, workflow, "DOCS_BASE_PATH: /plugin-kit-ai/docs/")
	mustContain(t, workflow, "pnpm run build:pages")
	mustContain(t, workflow, "path: .pages-dist")

	packageBody, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	pkg := string(packageBody)
	mustContain(t, pkg, `"build:pages": "node ./scripts/build-pages-artifact.mjs"`)

	scriptBody, err := os.ReadFile(filepath.Join(root, "scripts", "build-pages-artifact.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBody)
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

	robotsBody, err := os.ReadFile(filepath.Join(root, "server", "routes", "robots.txt.ts"))
	if err != nil {
		t.Fatal(err)
	}
	robots := string(robotsBody)
	mustContain(t, robots, `https://777genius.github.io/plugin-kit-ai/docs/sitemap.xml`)
}
