import path from "node:path";
import { extractCLI } from "./extractors/cli.mjs";
import { extractGoSDK } from "./extractors/go-sdk.mjs";
import { extractNodeRuntime } from "./extractors/node-runtime.mjs";
import { extractPythonRuntime } from "./extractors/python-runtime.mjs";
import { extractPlatformData } from "./extractors/platform.mjs";
import { generatedRoot, generatedRegistryPaths, runtimeRoot, sourceRoot, websiteRoot } from "./config/site.mjs";
import { copyTree, ensureDir, listMarkdownFiles, rimraf, writeFile, writeJson } from "./lib/fs.mjs";
import { readFrontmatter } from "./lib/site-model.mjs";

const locales = ["en", "ru"];

await rimraf(path.join(generatedRoot, "en"));
await rimraf(path.join(generatedRoot, "ru"));
await ensureDir(path.join(generatedRoot, "registries"));

const bundles = await Promise.all([
  extractCLI(),
  extractGoSDK(),
  extractNodeRuntime(),
  extractPythonRuntime(),
  extractPlatformData()
]);

const generatedEntities = bundles.flatMap((bundle) => bundle.entities);
const generatedPages = bundles.flatMap((bundle) => bundle.pages);

for (const page of generatedPages) {
  await writeFile(path.join(generatedRoot, page.relativePath), page.content);
}

const sourceEntities = await scanSourceEntities();
const allEntities = [...sourceEntities, ...generatedEntities].sort((a, b) =>
  a.canonicalId.localeCompare(b.canonicalId)
);
await writeJson(generatedRegistryPaths.entities, allEntities);
await writeJson(generatedRegistryPaths.sidebarsEn, buildSidebar("en", allEntities));
await writeJson(generatedRegistryPaths.sidebarsRu, buildSidebar("ru", allEntities));
await writeJson(generatedRegistryPaths.redirects, {});

await rimraf(runtimeRoot);
await ensureDir(runtimeRoot);
await copyTree(path.join(websiteRoot, "public"), path.join(runtimeRoot, "public"));
await copyTree(path.join(sourceRoot, "gateway"), runtimeRoot);
for (const locale of locales) {
  await copyTree(path.join(sourceRoot, locale), path.join(runtimeRoot, locale));
  await copyTree(path.join(generatedRoot, locale), path.join(runtimeRoot, locale));
}

async function scanSourceEntities() {
  const entities = [];
  for (const locale of locales) {
    const files = await listMarkdownFiles(path.join(sourceRoot, locale));
    for (const filePath of files) {
      const meta = await readFrontmatter(filePath);
      if (!meta.canonicalId) {
        continue;
      }
      const relative = path.relative(path.join(sourceRoot, locale), filePath).replace(/\\/g, "/");
      const existing = entities.find((entry) => entry.canonicalId === meta.canonicalId);
      const targetPath = `/${locale}/${relative.replace(/index\.md$/, "").replace(/\.md$/, "")}`;
      const localeKey = locale === "en" ? "pathEn" : "pathRu";
      if (existing) {
        existing[localeKey] = targetPath;
        continue;
      }
      entities.push({
        canonicalId: meta.canonicalId,
        kind: "page",
        surface: meta.section || "page",
        localeStrategy: "mirrored",
        title: meta.title || relative,
        summary: meta.description || "",
        stability: meta.stability || "public-stable",
        maturity: meta.maturity || "stable",
        publicVisibility: "public",
        sourceKind: "hand-authored",
        sourceRef: relative,
        pathEn: locale === "en" ? targetPath : "",
        pathRu: locale === "ru" ? targetPath : "",
        relatedIds: [],
        searchTerms: [meta.title || relative]
      });
    }
  }
  return entities;
}

function buildSidebar(locale, entities) {
  const prefix = `/${locale}/`;
  const labels = localeLabels(locale);
  const entityPath = (entry) => (locale === "en" ? entry.pathEn : entry.pathRu);
  const linkItem = (text, link) => ({ text, link });
  const pageLink = (canonicalId, fallback) => {
    const entry = entities.find((candidate) => candidate.canonicalId === canonicalId);
    return entry ? entityPath(entry) : fallback;
  };
  const section = (name, items) => [{ text: name, items }];
  const surfaceItems = (surface, formatter = (entry) => entry.title) =>
    entities
      .filter((entry) => entry.surface === surface && entry.kind !== "page")
      .map((entry) => ({
        text: formatter(entry),
        link: entityPath(entry)
      }))
      .sort((a, b) => a.text.localeCompare(b.text));

  const cliEntries = entities
    .filter((entry) => entry.surface === "cli" && entry.kind === "command")
    .sort((a, b) => a.title.localeCompare(b.title));
  const cliGroups = buildCliGroups(locale, cliEntries, entityPath);
  const goItems = surfaceItems("go-sdk");
  const nodeItems = surfaceItems("runtime-node");
  const pythonItems = surfaceItems("runtime-python");
  const platformItems = surfaceItems("platform-events");
  const capabilityItems = surfaceItems("capabilities");

  return {
    [`${prefix}guide/`]: [
      {
        text: labels.guideStart,
        items: [
          linkItem(labels.guideOverview, `${prefix}guide/`),
          linkItem(labels.installation, `${prefix}guide/installation`),
          linkItem(labels.quickstart, `${prefix}guide/quickstart`)
        ]
      },
      {
        text: labels.guideCoreIdea,
        items: [
          linkItem(labels.whatYouCanBuild, `${prefix}guide/what-you-can-build`),
          linkItem(labels.oneProjectMultipleTargets, `${prefix}guide/one-project-multiple-targets`),
          linkItem(labels.chooseTarget, `${prefix}guide/choose-a-target`)
        ]
      },
      {
        text: labels.guideBuild,
        items: [
          linkItem(labels.firstPlugin, `${prefix}guide/first-plugin`),
          linkItem(labels.teamReadyPlugin, `${prefix}guide/team-ready-plugin`),
          linkItem(labels.claudePlugin, `${prefix}guide/claude-plugin`),
          linkItem(labels.nodeTypescriptRuntime, `${prefix}guide/node-typescript-runtime`)
        ]
      },
      {
        text: labels.guideStarters,
        items: [
          linkItem(labels.starterTemplates, `${prefix}guide/starter-templates`),
          linkItem(labels.chooseStarterRepo, `${prefix}guide/choose-a-starter`),
          linkItem(labels.examplesAndRecipes, `${prefix}guide/examples-and-recipes`)
        ]
      },
      {
        text: labels.guideDelivery,
        items: [
          linkItem(labels.chooseDeliveryModel, `${prefix}guide/choose-delivery-model`),
          linkItem(labels.bundleHandoff, `${prefix}guide/bundle-handoff`),
          linkItem(labels.packageAndWorkspaceTargets, `${prefix}guide/package-and-workspace-targets`),
          linkItem(labels.migrateExistingConfig, `${prefix}guide/migrate-existing-config`)
        ]
      },
      {
        text: labels.guideOperate,
        items: [
          linkItem(labels.productionReadiness, `${prefix}guide/production-readiness`),
          linkItem(labels.ciIntegration, `${prefix}guide/ci-integration`)
        ]
      }
    ],
    [`${prefix}concepts/`]: [
      {
        text: labels.conceptsFoundation,
        items: [
          linkItem(labels.conceptsOverview, `${prefix}concepts/`),
          linkItem(labels.whyPluginKitAi, `${prefix}concepts/why-plugin-kit-ai`),
          linkItem(labels.managedProjectModel, `${prefix}concepts/managed-project-model`),
          linkItem(labels.authoringArchitecture, `${prefix}concepts/authoring-architecture`)
        ]
      },
      {
        text: labels.conceptsDecisions,
        items: [
          linkItem(labels.stabilityModel, `${prefix}concepts/stability-model`),
          linkItem(labels.targetModel, `${prefix}concepts/target-model`),
          linkItem(labels.choosingRuntime, `${prefix}concepts/choosing-runtime`)
        ]
      }
    ],
    [`${prefix}reference/`]: [
      {
        text: labels.referenceOperational,
        items: [
          linkItem(labels.referenceOverview, `${prefix}reference/`),
          linkItem(labels.installChannels, `${prefix}reference/install-channels`),
          linkItem(labels.versionAndCompatibility, `${prefix}reference/version-and-compatibility`),
          linkItem(labels.authoringWorkflow, `${prefix}reference/authoring-workflow`),
          linkItem(labels.repositoryStandard, `${prefix}reference/repository-standard`)
        ]
      },
      {
        text: labels.referenceSupport,
        items: [
          linkItem(labels.supportBoundary, `${prefix}reference/support-boundary`),
          linkItem(labels.targetSupport, `${prefix}reference/target-support`)
        ]
      },
      {
        text: labels.referenceHelp,
        items: [
          linkItem(labels.faq, `${prefix}reference/faq`),
          linkItem(labels.troubleshooting, `${prefix}reference/troubleshooting`),
          linkItem(labels.glossary, `${prefix}reference/glossary`)
        ]
      }
    ],
    [`${prefix}api/`]: section(labels.apiOverview, [
      linkItem(labels.apiOverview, `${prefix}api/`),
      linkItem(labels.cliReference, pageLink("page:api:cli:index", `${prefix}api/cli/`)),
      linkItem(labels.goSdk, pageLink("page:api:go-sdk:index", `${prefix}api/go-sdk/`)),
      linkItem(labels.nodeRuntime, pageLink("page:api:runtime-node:index", `${prefix}api/runtime-node/`)),
      linkItem(labels.pythonRuntime, pageLink("page:api:runtime-python:index", `${prefix}api/runtime-python/`)),
      linkItem(labels.platformEvents, pageLink("page:api:platform-events:index", `${prefix}api/platform-events/`)),
      linkItem(labels.capabilities, pageLink("page:api:capabilities:index", `${prefix}api/capabilities/`))
    ]),
    [`${prefix}api/cli/`]: [
      { text: labels.cliReference, items: [linkItem(labels.cliOverview, pageLink("page:api:cli:index", `${prefix}api/cli/`))] },
      ...cliGroups
    ],
    [`${prefix}api/go-sdk/`]: section(labels.goSdk, [
      linkItem(labels.goSdkOverview, pageLink("page:api:go-sdk:index", `${prefix}api/go-sdk/`)),
      ...goItems
    ]),
    [`${prefix}api/runtime-node/`]: section(labels.nodeRuntime, [
      linkItem(labels.nodeRuntimeOverview, pageLink("page:api:runtime-node:index", `${prefix}api/runtime-node/`)),
      ...nodeItems
    ]),
    [`${prefix}api/runtime-python/`]: section(labels.pythonRuntime, [
      linkItem(labels.pythonRuntimeOverview, pageLink("page:api:runtime-python:index", `${prefix}api/runtime-python/`)),
      ...pythonItems
    ]),
    [`${prefix}api/platform-events/`]: section(labels.platformEvents, [
      linkItem(labels.platformEventsOverview, pageLink("page:api:platform-events:index", `${prefix}api/platform-events/`)),
      ...platformItems
    ]),
    [`${prefix}api/capabilities/`]: section(labels.capabilities, [
      linkItem(labels.capabilitiesOverview, pageLink("page:api:capabilities:index", `${prefix}api/capabilities/`)),
      ...capabilityItems
    ]),
    [`${prefix}releases/`]: section(labels.releases, [
      linkItem(labels.releasesOverview, `${prefix}releases/`),
      linkItem("v1.0.6", `${prefix}releases/v1-0-6`),
      linkItem("v1.0.0", `${prefix}releases/v1-0-0`),
      linkItem("v1.0.4 Go SDK", `${prefix}releases/v1-0-4-go-sdk`)
    ])
  };
}

function buildCliGroups(locale, cliEntries, entityPath) {
  const labels = localeLabels(locale);
  const buckets = new Map([
    ["core", []],
    ["bundle", []],
    ["completion", []],
    ["skills", []]
  ]);

  for (const entry of cliEntries) {
    const parts = entry.title.split(" ");
    const family = parts[1];
    const groupKey = buckets.has(family) ? family : "core";
    const shortText = parts.length <= 1 ? entry.title : parts.slice(1).join(" ");
    buckets.get(groupKey).push({ text: shortText, link: entityPath(entry) });
  }

  return [
    { text: labels.cliCore, items: buckets.get("core") },
    { text: labels.cliBundle, items: buckets.get("bundle") },
    { text: labels.cliCompletion, items: buckets.get("completion") },
    { text: labels.cliSkills, items: buckets.get("skills") }
  ].filter((group) => group.items.length > 0);
}

function localeLabels(locale) {
  if (locale === "ru") {
    return {
      guide: "Гайды",
      guideStart: "Старт",
      guideCoreIdea: "Суть проекта",
      guideBuild: "Сборка плагина",
      guideStarters: "Starter'ы и примеры",
      guideDelivery: "Поставка и target'ы",
      guideOperate: "Продакшен и CI",
      guideOverview: "Обзор",
      quickstart: "Быстрый старт",
      installation: "Установка",
      whatYouCanBuild: "Что можно построить",
      oneProjectMultipleTargets: "Один проект, несколько target’ов",
      chooseTarget: "Выбор target",
      firstPlugin: "Соберите первый плагин",
      teamReadyPlugin: "Плагин для команды",
      claudePlugin: "Плагин для Claude",
      nodeTypescriptRuntime: "Node/TypeScript runtime",
      starterTemplates: "Стартовые шаблоны",
      examplesAndRecipes: "Примеры и рецепты",
      chooseStarterRepo: "Выбор стартового репозитория",
      chooseDeliveryModel: "Выбор модели поставки",
      bundleHandoff: "Bundle handoff",
      packageAndWorkspaceTargets: "Package и workspace targets",
      migrateExistingConfig: "Миграция существующей конфигурации",
      productionReadiness: "Готовность к продакшену",
      ciIntegration: "Интеграция с CI",
      concepts: "Концепции",
      conceptsFoundation: "Основа",
      conceptsDecisions: "Модели выбора",
      conceptsOverview: "Обзор",
      whyPluginKitAi: "Зачем plugin-kit-ai",
      managedProjectModel: "Модель управляемого проекта",
      authoringArchitecture: "Архитектура авторинга",
      stabilityModel: "Модель стабильности",
      targetModel: "Модель target’ов",
      choosingRuntime: "Выбор runtime",
      reference: "Справочник",
      referenceOperational: "Рабочий контур",
      referenceSupport: "Поддержка и границы",
      referenceHelp: "Помощь",
      referenceOverview: "Обзор",
      installChannels: "Каналы установки",
      versionAndCompatibility: "Политика версий и совместимости",
      authoringWorkflow: "Процесс авторинга",
      repositoryStandard: "Стандарт репозитория",
      supportBoundary: "Граница поддержки",
      targetSupport: "Поддержка Target'ов",
      faq: "Частые вопросы",
      troubleshooting: "Диагностика проблем",
      glossary: "Словарь терминов",
      apiOverview: "Обзор API",
      cliReference: "Справочник CLI",
      cliOverview: "Обзор CLI",
      cliCore: "Основные команды",
      cliBundle: "Bundle",
      cliCompletion: "Completion",
      cliSkills: "Skills",
      goSdk: "Go SDK",
      goSdkOverview: "Обзор Go SDK",
      nodeRuntime: "Node Runtime",
      nodeRuntimeOverview: "Обзор Node Runtime",
      pythonRuntime: "Python Runtime",
      pythonRuntimeOverview: "Обзор Python Runtime",
      platformEvents: "События платформ",
      platformEventsOverview: "Обзор событий",
      capabilities: "Capabilities",
      capabilitiesOverview: "Обзор возможностей",
      releases: "Релизы",
      releasesOverview: "Обзор"
    };
  }

  return {
    guide: "Guide",
    guideStart: "Start",
    guideCoreIdea: "Core Idea",
    guideBuild: "Build Plugins",
    guideStarters: "Starters And Examples",
    guideDelivery: "Delivery And Targets",
    guideOperate: "Production And CI",
    guideOverview: "Overview",
    quickstart: "Quickstart",
    installation: "Installation",
    whatYouCanBuild: "What You Can Build",
    oneProjectMultipleTargets: "One Project, Multiple Targets",
    chooseTarget: "Choose A Target",
    firstPlugin: "Build Your First Plugin",
    teamReadyPlugin: "Build A Team-Ready Plugin",
    claudePlugin: "Build A Claude Plugin",
    nodeTypescriptRuntime: "Node/TypeScript Runtime",
    starterTemplates: "Starter Templates",
    examplesAndRecipes: "Examples And Recipes",
    chooseStarterRepo: "Choose A Starter Repo",
    chooseDeliveryModel: "Choose Delivery Model",
    bundleHandoff: "Bundle Handoff",
    packageAndWorkspaceTargets: "Package And Workspace Targets",
    migrateExistingConfig: "Migrate Existing Config",
    productionReadiness: "Production Readiness",
    ciIntegration: "CI Integration",
    concepts: "Concepts",
    conceptsFoundation: "Foundation",
    conceptsDecisions: "Decision Models",
    conceptsOverview: "Overview",
    whyPluginKitAi: "Why plugin-kit-ai",
    managedProjectModel: "Managed Project Model",
    authoringArchitecture: "Authoring Architecture",
    stabilityModel: "Stability Model",
    targetModel: "Target Model",
    choosingRuntime: "Choosing Runtime",
    reference: "Reference",
    referenceOperational: "Operational Reference",
    referenceSupport: "Support And Boundaries",
    referenceHelp: "Help",
    referenceOverview: "Overview",
    installChannels: "Install Channels",
    versionAndCompatibility: "Version And Compatibility Policy",
    authoringWorkflow: "Authoring Workflow",
    repositoryStandard: "Repository Standard",
    supportBoundary: "Support Boundary",
    targetSupport: "Target Support",
    faq: "FAQ",
    troubleshooting: "Troubleshooting",
    glossary: "Glossary",
    apiOverview: "API Overview",
    cliReference: "CLI Reference",
    cliOverview: "Overview",
    cliCore: "Core Commands",
    cliBundle: "Bundle",
    cliCompletion: "Completion",
    cliSkills: "Skills",
    goSdk: "Go SDK",
    goSdkOverview: "Overview",
    nodeRuntime: "Node Runtime",
    nodeRuntimeOverview: "Overview",
    pythonRuntime: "Python Runtime",
    pythonRuntimeOverview: "Overview",
    platformEvents: "Platform Events",
    platformEventsOverview: "Overview",
    capabilities: "Capabilities",
    capabilitiesOverview: "Overview",
    releases: "Releases",
    releasesOverview: "Overview"
  };
}
