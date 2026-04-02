import fs from "node:fs/promises";
import path from "node:path";
import { docsBaseUrl, websiteRoot } from "../config/site.mjs";
import { listMarkdownFiles } from "../lib/fs.mjs";

const distRoot = path.join(websiteRoot, "dist");
const htmlFiles = (await listHtmlFiles(distRoot)).sort();
const editPrefix = `https://github.com/777genius/plugin-kit-ai/edit/main/website/source/`;

let hasError = false;
for (const filePath of htmlFiles) {
  const body = await fs.readFile(filePath, "utf8");
  if (body.includes("maintainer-docs")) {
    console.error(`Internal docs leaked into built output: ${filePath}`);
    hasError = true;
  }
  if (/href="\/(en|ru)\//.test(body) || /src="\/assets\//.test(body)) {
    console.error(`Root-relative path detected in built output: ${filePath}`);
    hasError = true;
  }
}

const gateway = await fs.readFile(path.join(distRoot, "index.html"), "utf8");
if (!gateway.includes("noindex,follow")) {
  console.error("Gateway page is missing robots noindex,follow.");
  hasError = true;
}

const notFound = await fs.readFile(path.join(distRoot, "404.html"), "utf8");
if (!notFound.includes("noindex,follow")) {
  console.error("404 page is missing robots noindex,follow.");
  hasError = true;
}

const sitemap = await fs.readFile(path.join(distRoot, "sitemap.xml"), "utf8");
if (sitemap.includes(`<loc>${docsBaseUrl}</loc>`)) {
  console.error("Gateway root leaked into sitemap.xml.");
  hasError = true;
}

const robots = await fs.readFile(path.join(distRoot, "robots.txt"), "utf8");
if (!robots.includes(`Sitemap: ${new URL("sitemap.xml", docsBaseUrl).toString()}`)) {
  console.error("robots.txt is missing the sitemap declaration.");
  hasError = true;
}

const handAuthoredHome = await fs.readFile(path.join(distRoot, "en", "index.html"), "utf8");
if (!handAuthoredHome.includes(`${editPrefix}en/index.md`)) {
  console.error("Hand-authored EN home page is missing its edit link.");
  hasError = true;
}

const generatedCli = await fs.readFile(path.join(distRoot, "en", "api", "cli", "plugin-kit-ai.html"), "utf8");
if (generatedCli.includes(`${editPrefix}en/api/cli/plugin-kit-ai.md`)) {
  console.error("Generated CLI page rendered a hand-authored edit link.");
  hasError = true;
}
if (!generatedCli.includes(">Source<")) {
  console.error("Generated CLI page is missing the Source CTA.");
  hasError = true;
}

const latestRelease = await fs.readFile(path.join(distRoot, "en", "releases", "v1-0-6.html"), "utf8");
if (!latestRelease.includes("Why This Release Matters")) {
  console.error("Latest public release page is missing its expected headline content.");
  hasError = true;
}

const productionReadiness = await fs.readFile(path.join(distRoot, "en", "guide", "production-readiness.html"), "utf8");
if (!productionReadiness.includes("Pick The Right Path On Purpose")) {
  console.error("Production Readiness page is missing its expected checklist structure.");
  hasError = true;
}

const ciIntegration = await fs.readFile(path.join(distRoot, "en", "guide", "ci-integration.html"), "utf8");
if (!ciIntegration.includes("The Minimal CI Gate")) {
  console.error("CI Integration page is missing its expected CI gate section.");
  hasError = true;
}

const teamReadyPlugin = await fs.readFile(path.join(distRoot, "en", "guide", "team-ready-plugin.html"), "utf8");
if (!teamReadyPlugin.includes("The repo is ready when another teammate can clone it")) {
  console.error("Team-Ready Plugin page is missing its expected handoff rule.");
  hasError = true;
}

const examplesAndRecipes = await fs.readFile(path.join(distRoot, "en", "guide", "examples-and-recipes.html"), "utf8");
if (!examplesAndRecipes.includes("Production Plugin Examples")) {
  console.error("Examples And Recipes page is missing its expected examples section.");
  hasError = true;
}

const chooseStarter = await fs.readFile(path.join(distRoot, "en", "guide", "choose-a-starter.html"), "utf8");
if (!chooseStarter.includes("Starter Matrix")) {
  console.error("Choose A Starter page is missing its expected starter-matrix section.");
  hasError = true;
}

const chooseDeliveryModel = await fs.readFile(path.join(distRoot, "en", "guide", "choose-delivery-model.html"), "utf8");
if (!chooseDeliveryModel.includes("The Two Modes")) {
  console.error("Choose Delivery Model page is missing its expected mode-comparison section.");
  hasError = true;
}

const bundleHandoff = await fs.readFile(path.join(distRoot, "en", "guide", "bundle-handoff.html"), "utf8");
if (!bundleHandoff.includes("What It Covers")) {
  console.error("Bundle Handoff page is missing its expected capability-coverage section.");
  hasError = true;
}

const packageAndWorkspaceTargets = await fs.readFile(
  path.join(distRoot, "en", "guide", "package-and-workspace-targets.html"),
  "utf8"
);
if (!packageAndWorkspaceTargets.includes("The Short Rule")) {
  console.error("Package And Workspace Targets page is missing its expected decision-rule section.");
  hasError = true;
}

const whatYouCanBuild = await fs.readFile(path.join(distRoot, "en", "guide", "what-you-can-build.html"), "utf8");
if (!whatYouCanBuild.includes("One Repo, Many Supported Outputs")) {
  console.error("What You Can Build page is missing its expected product-shape section.");
  hasError = true;
}

const oneProjectMultipleTargets = await fs.readFile(
  path.join(distRoot, "en", "guide", "one-project-multiple-targets.html"),
  "utf8"
);
if (!oneProjectMultipleTargets.includes("The Short Rule")) {
  console.error("One Project, Multiple Targets page is missing its expected mental-model section.");
  hasError = true;
}

const chooseTarget = await fs.readFile(path.join(distRoot, "en", "guide", "choose-a-target.html"), "utf8");
if (!chooseTarget.includes("Target Directory")) {
  console.error("Choose A Target page is missing its expected target-decision section.");
  hasError = true;
}

const authoringArchitecture = await fs.readFile(path.join(distRoot, "en", "concepts", "authoring-architecture.html"), "utf8");
if (!authoringArchitecture.includes("The Core Shape")) {
  console.error("Authoring Architecture page is missing its expected core-shape section.");
  hasError = true;
}

const choosingRuntime = await fs.readFile(path.join(distRoot, "en", "concepts", "choosing-runtime.html"), "utf8");
if (!choosingRuntime.includes("Safe Default Matrix")) {
  console.error("Choosing Runtime page is missing its expected decision matrix.");
  hasError = true;
}

const targetModel = await fs.readFile(path.join(distRoot, "en", "concepts", "target-model.html"), "utf8");
if (!targetModel.includes("Quick Rule")) {
  console.error("Target Model page is missing its expected quick-rule section.");
  hasError = true;
}

const glossary = await fs.readFile(path.join(distRoot, "en", "reference", "glossary.html"), "utf8");
if (!glossary.includes("Authored State")) {
  console.error("Glossary page is missing its expected term content.");
  hasError = true;
}

const repositoryStandard = await fs.readFile(path.join(distRoot, "en", "reference", "repository-standard.html"), "utf8");
if (!repositoryStandard.includes("The Main Rule")) {
  console.error("Repository Standard page is missing its expected main-rule section.");
  hasError = true;
}

const supportBoundary = await fs.readFile(path.join(distRoot, "en", "reference", "support-boundary.html"), "utf8");
if (!supportBoundary.includes("Safe Defaults")) {
  console.error("Support Boundary page is missing its expected safety framing.");
  hasError = true;
}

if (hasError) {
  process.exit(1);
}

async function listHtmlFiles(rootDir) {
  const out = [];
  await walk(rootDir, out);
  return out;
}

async function walk(currentDir, out) {
  const entries = await fs.readdir(currentDir, { withFileTypes: true });
  for (const entry of entries) {
    const currentPath = path.join(currentDir, entry.name);
    if (entry.isDirectory()) {
      await walk(currentPath, out);
      continue;
    }
    if (entry.name.endsWith(".html")) {
      out.push(currentPath);
    }
  }
}
