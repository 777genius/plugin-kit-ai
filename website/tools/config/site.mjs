import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export const websiteRoot = path.resolve(__dirname, "..", "..");
export const repoRoot = path.resolve(websiteRoot, "..");
export const sourceRoot = path.join(websiteRoot, "source");
export const generatedRoot = path.join(websiteRoot, "generated");
export const runtimeRoot = path.join(websiteRoot, ".site");
export const docsToolsRoot = path.join(websiteRoot, ".docs-tools");

export const locales = [
  { code: "en", label: "English", lang: "en-US" },
  { code: "ru", label: "Русский", lang: "ru-RU" }
];

export const publicGoPackages = [
  { id: "root", importPath: "github.com/777genius/plugin-kit-ai/sdk", relativePath: "sdk" },
  { id: "claude", importPath: "github.com/777genius/plugin-kit-ai/sdk/claude", relativePath: "sdk/claude" },
  { id: "codex", importPath: "github.com/777genius/plugin-kit-ai/sdk/codex", relativePath: "sdk/codex" },
  { id: "platformmeta", importPath: "github.com/777genius/plugin-kit-ai/sdk/platformmeta", relativePath: "sdk/platformmeta" }
];

export const docsHostname = process.env.DOCS_HOSTNAME || "https://777genius.github.io";
export const docsBasePath = process.env.DOCS_BASE_PATH || "/plugin-kit-ai/docs/";
export const docsBaseUrl = new URL(docsBasePath, docsHostname).toString();

export const generatedRegistryPaths = {
  entities: path.join(generatedRoot, "registries", "entities.json"),
  sidebarsEn: path.join(generatedRoot, "registries", "sidebars.en.json"),
  sidebarsRu: path.join(generatedRoot, "registries", "sidebars.ru.json"),
  redirects: path.join(generatedRoot, "registries", "redirects.json")
};

export const sourceRefs = {
  supportMatrix: "docs/generated/support_matrix.md",
  targetSupportMatrix: "docs/generated/target_support_matrix.md",
  nodeRuntime: "npm/plugin-kit-ai-runtime",
  pythonRuntime: "python/plugin-kit-ai-runtime/src/plugin_kit_ai_runtime/__init__.py"
};

export function repoBrowserUrl(sourceRef) {
  if (!sourceRef) {
    return "";
  }
  if (sourceRef.startsWith("cli:")) {
    return "https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai";
  }

  const mode = /\.[a-z0-9]+$/i.test(sourceRef) ? "blob" : "tree";
  return `https://github.com/777genius/plugin-kit-ai/${mode}/main/${sourceRef}`;
}
