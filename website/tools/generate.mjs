import path from "node:path";
import { extractCLI } from "./extractors/cli.mjs";
import { extractGoSDK } from "./extractors/go-sdk.mjs";
import { extractNodeRuntime } from "./extractors/node-runtime.mjs";
import { extractPythonRuntime } from "./extractors/python-runtime.mjs";
import { extractPlatformData } from "./extractors/platform.mjs";
import { generatedRoot, generatedRegistryPaths, runtimeRoot, sourceRoot, websiteRoot } from "./config/site.mjs";
import { copyTree, ensureDir, listMarkdownFiles, rimraf, writeFile, writeJson } from "./lib/fs.mjs";
import { readFrontmatter } from "./lib/site-model.mjs";

const sourceLocales = ["en", "ru"];
const docsLocales = ["en", "ru", "es", "fr", "zh"];
const mirroredGeneratedLocales = ["es", "fr", "zh"];

for (const locale of docsLocales) {
  await rimraf(path.join(generatedRoot, locale));
}
await ensureDir(path.join(generatedRoot, "registries"));

const bundles = await Promise.all([
  extractCLI(),
  extractGoSDK(),
  extractNodeRuntime(),
  extractPythonRuntime(),
  extractPlatformData()
]);

const generatedEntities = bundles.flatMap((bundle) => bundle.entities);
const baseGeneratedPages = bundles.flatMap((bundle) => bundle.pages);
const generatedPages = [
  ...baseGeneratedPages,
  ...baseGeneratedPages
    .filter((page) => page.locale === "en")
    .flatMap((page) => mirroredGeneratedLocales.map((locale) => mirrorGeneratedPage(page, locale)))
];

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
await writeJson(path.join(generatedRoot, "registries", "sidebars.es.json"), buildSidebar("es", allEntities));
await writeJson(path.join(generatedRoot, "registries", "sidebars.fr.json"), buildSidebar("fr", allEntities));
await writeJson(path.join(generatedRoot, "registries", "sidebars.zh.json"), buildSidebar("zh", allEntities));
await writeJson(generatedRegistryPaths.redirects, {});

await rimraf(runtimeRoot);
await ensureDir(runtimeRoot);
await copyTree(path.join(websiteRoot, "public"), path.join(runtimeRoot, "public"));
await copyTree(path.join(sourceRoot, "gateway"), runtimeRoot);
for (const locale of docsLocales) {
  await copyTree(path.join(sourceRoot, locale), path.join(runtimeRoot, locale));
  await copyTree(path.join(generatedRoot, locale), path.join(runtimeRoot, locale));
}

async function scanSourceEntities() {
  const entities = [];
  for (const locale of sourceLocales) {
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
  const entityPath = (entry) => {
    if (locale === "en") {
      return entry.pathEn;
    }
    if (locale === "ru") {
      return entry.pathRu;
    }
    return entry.pathEn ? entry.pathEn.replace(/^\/en\//, `/${locale}/`) : "";
  };
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
          linkItem(labels.pythonRuntimeGuide, `${prefix}guide/python-runtime`),
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
          linkItem(labels.howToPublishPlugins, `${prefix}guide/how-to-publish-plugins`)
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
      linkItem("v1.1.0", `${prefix}releases/v1-1-0`),
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
  if (locale === "es") {
    return {
      guide: "Guía",
      guideStart: "Inicio",
      guideCoreIdea: "Idea central",
      guideBuild: "Crear plugins",
      guideStarters: "Starters y ejemplos",
      guideDelivery: "Entrega y targets",
      guideOperate: "Producción y CI",
      guideOverview: "Resumen",
      quickstart: "Inicio rápido",
      installation: "Instalación",
      whatYouCanBuild: "Qué puedes crear",
      oneProjectMultipleTargets: "Un proyecto, múltiples targets",
      chooseTarget: "Elegir un target",
      firstPlugin: "Crea tu primer plugin",
      pythonRuntimeGuide: "Plugin Python runtime",
      teamReadyPlugin: "Plugin listo para equipos",
      claudePlugin: "Plugin para Claude",
      nodeTypescriptRuntime: "Runtime Node/TypeScript",
      starterTemplates: "Plantillas starter",
      examplesAndRecipes: "Ejemplos y recetas",
      chooseStarterRepo: "Elegir un starter repo",
      chooseDeliveryModel: "Elegir modelo de entrega",
      bundleHandoff: "Bundle handoff",
      packageAndWorkspaceTargets: "Packages e integración",
      howToPublishPlugins: "Cómo publicar plugins",
      productionReadiness: "Preparación para producción",
      ciIntegration: "Integración con CI",
      concepts: "Conceptos",
      conceptsFoundation: "Base",
      conceptsDecisions: "Modelos de decisión",
      conceptsOverview: "Resumen",
      whyPluginKitAi: "Por qué plugin-kit-ai",
      managedProjectModel: "Cómo funciona plugin-kit-ai",
      authoringArchitecture: "Código fuente y outputs",
      stabilityModel: "Modelo de estabilidad",
      targetModel: "Modelo de targets",
      choosingRuntime: "Elegir runtime",
      reference: "Referencia",
      referenceOperational: "Referencia operativa",
      referenceSupport: "Soporte y límites",
      referenceHelp: "Ayuda",
      referenceOverview: "Resumen",
      installChannels: "Canales de instalación",
      versionAndCompatibility: "Versiones y compatibilidad",
      authoringWorkflow: "Flujo de autoría",
      repositoryStandard: "Estándar del repositorio",
      supportBoundary: "Límite de soporte",
      targetSupport: "Soporte de targets",
      faq: "FAQ",
      troubleshooting: "Resolución de problemas",
      glossary: "Glosario",
      apiOverview: "Resumen de API",
      cliReference: "Referencia CLI",
      cliOverview: "Resumen",
      cliCore: "Comandos principales",
      cliBundle: "Bundle",
      cliCompletion: "Completion",
      cliSkills: "Skills",
      goSdk: "Go SDK",
      goSdkOverview: "Resumen",
      nodeRuntime: "Node Runtime",
      nodeRuntimeOverview: "Resumen",
      pythonRuntime: "Python Runtime",
      pythonRuntimeOverview: "Resumen",
      platformEvents: "Eventos de plataforma",
      platformEventsOverview: "Resumen",
      capabilities: "Capacidades",
      capabilitiesOverview: "Resumen",
      releases: "Lanzamientos",
      releasesOverview: "Resumen"
    };
  }

  if (locale === "fr") {
    return {
      guide: "Guide",
      guideStart: "Démarrer",
      guideCoreIdea: "Idée centrale",
      guideBuild: "Créer des plugins",
      guideStarters: "Starters et exemples",
      guideDelivery: "Livraison et targets",
      guideOperate: "Production et CI",
      guideOverview: "Vue d'ensemble",
      quickstart: "Démarrage rapide",
      installation: "Installation",
      whatYouCanBuild: "Ce que vous pouvez créer",
      oneProjectMultipleTargets: "Un projet, plusieurs targets",
      chooseTarget: "Choisir un target",
      firstPlugin: "Créer votre premier plugin",
      pythonRuntimeGuide: "Plugin Python runtime",
      teamReadyPlugin: "Plugin prêt pour l'équipe",
      claudePlugin: "Plugin Claude",
      nodeTypescriptRuntime: "Runtime Node/TypeScript",
      starterTemplates: "Templates starter",
      examplesAndRecipes: "Exemples et recettes",
      chooseStarterRepo: "Choisir un starter repo",
      chooseDeliveryModel: "Choisir le modèle de livraison",
      bundleHandoff: "Bundle handoff",
      packageAndWorkspaceTargets: "Packages et intégration",
      howToPublishPlugins: "Publier des plugins",
      productionReadiness: "Préparation production",
      ciIntegration: "Intégration CI",
      concepts: "Concepts",
      conceptsFoundation: "Fondations",
      conceptsDecisions: "Modèles de décision",
      conceptsOverview: "Vue d'ensemble",
      whyPluginKitAi: "Pourquoi plugin-kit-ai",
      managedProjectModel: "Comment fonctionne plugin-kit-ai",
      authoringArchitecture: "Sources et outputs",
      stabilityModel: "Modèle de stabilité",
      targetModel: "Modèle de targets",
      choosingRuntime: "Choisir le runtime",
      reference: "Référence",
      referenceOperational: "Référence opérationnelle",
      referenceSupport: "Support et limites",
      referenceHelp: "Aide",
      referenceOverview: "Vue d'ensemble",
      installChannels: "Canaux d'installation",
      versionAndCompatibility: "Versions et compatibilité",
      authoringWorkflow: "Workflow d'authoring",
      repositoryStandard: "Standard du dépôt",
      supportBoundary: "Périmètre de support",
      targetSupport: "Support des targets",
      faq: "FAQ",
      troubleshooting: "Dépannage",
      glossary: "Glossaire",
      apiOverview: "Vue d'ensemble API",
      cliReference: "Référence CLI",
      cliOverview: "Vue d'ensemble",
      cliCore: "Commandes principales",
      cliBundle: "Bundle",
      cliCompletion: "Completion",
      cliSkills: "Skills",
      goSdk: "Go SDK",
      goSdkOverview: "Vue d'ensemble",
      nodeRuntime: "Node Runtime",
      nodeRuntimeOverview: "Vue d'ensemble",
      pythonRuntime: "Python Runtime",
      pythonRuntimeOverview: "Vue d'ensemble",
      platformEvents: "Événements de plateforme",
      platformEventsOverview: "Vue d'ensemble",
      capabilities: "Capacités",
      capabilitiesOverview: "Vue d'ensemble",
      releases: "Versions",
      releasesOverview: "Vue d'ensemble"
    };
  }

  if (locale === "zh") {
    return {
      guide: "指南",
      guideStart: "开始",
      guideCoreIdea: "核心思路",
      guideBuild: "构建插件",
      guideStarters: "Starter 与示例",
      guideDelivery: "交付与 targets",
      guideOperate: "生产与 CI",
      guideOverview: "总览",
      quickstart: "快速开始",
      installation: "安装",
      whatYouCanBuild: "你可以构建什么",
      oneProjectMultipleTargets: "一个项目，多个 targets",
      chooseTarget: "选择 target",
      firstPlugin: "构建第一个插件",
      pythonRuntimeGuide: "Python runtime 插件",
      teamReadyPlugin: "团队可用插件",
      claudePlugin: "Claude 插件",
      nodeTypescriptRuntime: "Node/TypeScript Runtime",
      starterTemplates: "Starter 模板",
      examplesAndRecipes: "示例与实践",
      chooseStarterRepo: "选择 starter repo",
      chooseDeliveryModel: "选择交付模型",
      bundleHandoff: "Bundle 交付",
      packageAndWorkspaceTargets: "Packages 与集成配置",
      howToPublishPlugins: "如何发布插件",
      productionReadiness: "生产准备",
      ciIntegration: "CI 集成",
      concepts: "概念",
      conceptsFoundation: "基础",
      conceptsDecisions: "决策模型",
      conceptsOverview: "总览",
      whyPluginKitAi: "为什么选择 plugin-kit-ai",
      managedProjectModel: "plugin-kit-ai 如何工作",
      authoringArchitecture: "源码与 outputs",
      stabilityModel: "稳定性模型",
      targetModel: "Target 模型",
      choosingRuntime: "选择 runtime",
      reference: "参考",
      referenceOperational: "操作参考",
      referenceSupport: "支持与边界",
      referenceHelp: "帮助",
      referenceOverview: "总览",
      installChannels: "安装渠道",
      versionAndCompatibility: "版本与兼容性",
      authoringWorkflow: "编写流程",
      repositoryStandard: "仓库标准",
      supportBoundary: "支持边界",
      targetSupport: "Target 支持",
      faq: "FAQ",
      troubleshooting: "故障排查",
      glossary: "术语表",
      apiOverview: "API 总览",
      cliReference: "CLI 参考",
      cliOverview: "总览",
      cliCore: "核心命令",
      cliBundle: "Bundle",
      cliCompletion: "Completion",
      cliSkills: "Skills",
      goSdk: "Go SDK",
      goSdkOverview: "总览",
      nodeRuntime: "Node Runtime",
      nodeRuntimeOverview: "总览",
      pythonRuntime: "Python Runtime",
      pythonRuntimeOverview: "总览",
      platformEvents: "平台事件",
      platformEventsOverview: "总览",
      capabilities: "Capabilities",
      capabilitiesOverview: "总览",
      releases: "发布",
      releasesOverview: "总览"
    };
  }

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
      pythonRuntimeGuide: "Python runtime",
      teamReadyPlugin: "Плагин для команды",
      claudePlugin: "Плагин для Claude",
      nodeTypescriptRuntime: "Node/TypeScript runtime",
      starterTemplates: "Стартовые шаблоны",
      examplesAndRecipes: "Примеры и рецепты",
      chooseStarterRepo: "Выбор стартового репозитория",
      chooseDeliveryModel: "Выбор модели поставки",
      bundleHandoff: "Bundle handoff",
      packageAndWorkspaceTargets: "Package и workspace targets",
      howToPublishPlugins: "Как публиковать плагины",
      productionReadiness: "Готовность к продакшену",
      ciIntegration: "Интеграция с CI",
      concepts: "Концепции",
      conceptsFoundation: "Основа",
      conceptsDecisions: "Модели выбора",
      conceptsOverview: "Обзор",
      whyPluginKitAi: "Зачем plugin-kit-ai",
      managedProjectModel: "Как работает plugin-kit-ai",
      authoringArchitecture: "Исходники и generated outputs",
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
    pythonRuntimeGuide: "Build A Python Runtime Plugin",
    teamReadyPlugin: "Build A Team-Ready Plugin",
    claudePlugin: "Build A Claude Plugin",
    nodeTypescriptRuntime: "Node/TypeScript Runtime",
    starterTemplates: "Starter Templates",
    examplesAndRecipes: "Examples And Recipes",
    chooseStarterRepo: "Choose A Starter Repo",
    chooseDeliveryModel: "Choose Delivery Model",
    bundleHandoff: "Bundle Handoff",
    packageAndWorkspaceTargets: "Packages And Integration Setup",
    howToPublishPlugins: "How To Publish Plugins",
    productionReadiness: "Production Readiness",
    ciIntegration: "CI Integration",
    concepts: "Concepts",
    conceptsFoundation: "Foundation",
    conceptsDecisions: "Decision Models",
    conceptsOverview: "Overview",
    whyPluginKitAi: "Why plugin-kit-ai",
    managedProjectModel: "How plugin-kit-ai Works",
    authoringArchitecture: "Project Source And Outputs",
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

function mirrorGeneratedPage(page, locale) {
  const relativePath = page.relativePath.replace(/^en\//, `${locale}/`);
  const content = localizeMirroredGeneratedContent(
    page.content
      .replace(/\blocale:\s*"en"/, `locale: "${locale}"`)
      .replaceAll("/en/", `/${locale}/`),
    locale
  );

  return {
    ...page,
    locale,
    relativePath,
    content
  };
}

function localizeMirroredGeneratedContent(content, locale) {
  const replacements = mirroredGeneratedReplacements(locale);
  return replacements.reduce((acc, [from, to]) => acc.replaceAll(from, to), content);
}

function mirroredGeneratedReplacements(locale) {
  if (locale === "es") {
    return [
      ["CLI Reference", "Referencia CLI"],
      ["Generated from the live Cobra command tree.", "Generado a partir del árbol real de comandos Cobra."],
      ["Generated CLI reference", "Referencia CLI generada"],
      ["Core Commands", "Comandos principales"],
      ["Generated via pydoc-markdown.", "Generado mediante pydoc-markdown."],
      ["Generated from the public Go package via gomarkdoc.", "Generado desde el paquete público de Go mediante gomarkdoc."],
      ["Import path", "Ruta de importación"],
      ["Platform Events", "Eventos de plataforma"],
      ["Capabilities", "Capacidades"],
      ["Target Support", "Soporte de targets"],
      ["Generated via TypeDoc and typedoc-plugin-markdown.", "Generado mediante TypeDoc y typedoc-plugin-markdown."],
      ["Generated Node runtime pages:", "Páginas generadas de Node runtime:"],
      ["Generated Python runtime reference", "Referencia generada de Python runtime"],
      ["Generated Go SDK package reference", "Referencia generada del paquete Go SDK"],
      ["Generated platform event reference", "Referencia generada de eventos de plataforma"],
      ["Generated capability reference", "Referencia generada de capacidades"],
      ["Generated target support summary", "Resumen generado de soporte de targets"],
      ["Generated Node runtime reference", "Referencia generada de Node runtime"],
      ["Generated runtime helper API", "API generada de helpers de runtime"]
    ];
  }
  if (locale === "fr") {
    return [
      ["CLI Reference", "Référence CLI"],
      ["Generated from the live Cobra command tree.", "Généré à partir de l'arbre réel de commandes Cobra."],
      ["Generated CLI reference", "Référence CLI générée"],
      ["Core Commands", "Commandes principales"],
      ["Generated via pydoc-markdown.", "Généré via pydoc-markdown."],
      ["Generated from the public Go package via gomarkdoc.", "Généré à partir du package Go public via gomarkdoc."],
      ["Import path", "Chemin d'import"],
      ["Platform Events", "Événements de plateforme"],
      ["Capabilities", "Capacités"],
      ["Target Support", "Support des targets"],
      ["Generated via TypeDoc and typedoc-plugin-markdown.", "Généré via TypeDoc et typedoc-plugin-markdown."],
      ["Generated Node runtime pages:", "Pages Node runtime générées :"],
      ["Generated Python runtime reference", "Référence Python runtime générée"],
      ["Generated Go SDK package reference", "Référence générée du package Go SDK"],
      ["Generated platform event reference", "Référence générée des événements de plateforme"],
      ["Generated capability reference", "Référence générée des capacités"],
      ["Generated target support summary", "Résumé généré du support des targets"],
      ["Generated Node runtime reference", "Référence Node runtime générée"],
      ["Generated runtime helper API", "API générée des helpers runtime"]
    ];
  }
  if (locale === "zh") {
    return [
      ["CLI Reference", "CLI 参考"],
      ["Generated from the live Cobra command tree.", "由实际的 Cobra 命令树生成。"],
      ["Generated CLI reference", "生成的 CLI 参考"],
      ["Core Commands", "核心命令"],
      ["Generated via pydoc-markdown.", "通过 pydoc-markdown 生成。"],
      ["Generated from the public Go package via gomarkdoc.", "通过 gomarkdoc 从公开 Go package 生成。"],
      ["Import path", "导入路径"],
      ["Platform Events", "平台事件"],
      ["Capabilities", "Capabilities"],
      ["Target Support", "Target 支持"],
      ["Generated via TypeDoc and typedoc-plugin-markdown.", "通过 TypeDoc 和 typedoc-plugin-markdown 生成。"],
      ["Generated Node runtime pages:", "生成的 Node runtime 页面："],
      ["Generated Python runtime reference", "生成的 Python runtime 参考"],
      ["Generated Go SDK package reference", "生成的 Go SDK package 参考"],
      ["Generated platform event reference", "生成的平台事件参考"],
      ["Generated capability reference", "生成的 capability 参考"],
      ["Generated target support summary", "生成的 target 支持摘要"],
      ["Generated Node runtime reference", "生成的 Node runtime 参考"],
      ["Generated runtime helper API", "生成的 runtime helper API"]
    ];
  }
  return [];
}
