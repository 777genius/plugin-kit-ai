<script setup lang="ts">
import { computed } from 'vue';
import { mdiArrowLeft, mdiOpenInNew } from '@mdi/js';
import { getPluginBySlug } from '~/data/content';
import type { LocaleCode } from '~/data/i18n';

const route = useRoute();
const localePath = useLocalePath();
const { locale, t } = useI18n();

const pluginAccentMap: Record<string, string> = {
  context7: '#00f0ff',
  gitlab: '#ff7a1a',
  github: '#7c9dff',
  firebase: '#ffd54f',
  linear: '#7df9ff',
  supabase: '#3ecf8e',
  greptile: '#70b5ff',
};

const slug = computed(() => String(route.params.slug));
const plugin = computed(() => getPluginBySlug(locale.value as LocaleCode, slug.value));

if (!plugin.value) {
  throw createError({
    statusCode: 404,
    statusMessage: 'Plugin not found',
  });
}

const accent = computed(() => pluginAccentMap[plugin.value?.slug ?? ''] ?? '#00f0ff');
const backToCatalogPath = computed(() => localePath('/plugins'));
const detailTitle = computed(() =>
  t('meta.pluginDetailTitle', { plugin: plugin.value?.title ?? '' }),
);
const detailDescription = computed(() =>
  t('meta.pluginDetailDescription', {
    plugin: plugin.value?.title ?? '',
    tagline: plugin.value?.tagline ?? '',
  }),
);

usePageSeo(detailTitle, detailDescription, { translate: false });
</script>

<template>
  <div v-if="plugin" class="plugin-detail" :style="{ '--accent': accent }">
    <PageBackground />

    <section class="plugin-detail__hero section">
      <v-container>
        <div class="plugin-detail__hero-shell">
          <div class="plugin-detail__hero-copy">
            <div class="plugin-detail__headline">
              <span
                :class="[
                  'plugin-detail__logo-wrap',
                  `plugin-detail__logo-wrap--${plugin.logoSurface ?? 'default'}`,
                ]"
              >
                <img
                  :src="plugin.logoSrc"
                  :alt="plugin.logoAlt"
                  class="plugin-detail__logo"
                  loading="eager"
                  decoding="async"
                >
              </span>
              <div>
                <h1 class="plugin-detail__title">{{ plugin.title }}</h1>
                <p class="plugin-detail__tagline">{{ plugin.tagline }}</p>
              </div>
            </div>

            <div class="plugin-detail__chips">
              <span class="plugin-detail__status">{{ plugin.status }}</span>
              <span
                v-for="category in plugin.categories"
                :key="category"
                class="plugin-detail__category"
              >
                {{ t(`plugins.categories.${category}`) }}
              </span>
            </div>

            <p class="plugin-detail__summary">{{ plugin.description }}</p>

            <div class="plugin-detail__actions">
              <v-btn
                :href="plugin.href"
                target="_blank"
                rel="noreferrer noopener"
                size="large"
                class="plugin-detail__primary-cta"
              >
                {{ t('plugins.openRepository') }}
                <v-icon :icon="mdiOpenInNew" end size="18" />
              </v-btn>
              <v-btn
                :to="backToCatalogPath"
                variant="outlined"
                size="large"
                class="plugin-detail__secondary-cta"
              >
                <v-icon :icon="mdiArrowLeft" start size="18" />
                {{ t('plugins.backToCatalog') }}
              </v-btn>
            </div>
          </div>

          <div class="plugin-detail__summary-card">
            <p class="plugin-detail__summary-eyebrow">
              {{ t('plugins.detailSummaryEyebrow') }}
            </p>

            <div class="plugin-detail__summary-block">
              <div class="plugin-detail__summary-title">{{ t('plugins.supports') }}</div>
              <div class="plugin-detail__badges">
                <AgentBadge
                  v-for="badge in plugin.badges"
                  :key="badge"
                  :label="badge"
                  tone="detail"
                />
              </div>
            </div>

            <div class="plugin-detail__summary-block">
              <div class="plugin-detail__summary-title">{{ t('plugins.categoriesTitle') }}</div>
              <ul class="plugin-detail__list">
                <li
                  v-for="category in plugin.categories"
                  :key="category"
                  class="plugin-detail__list-item"
                >
                  {{ t(`plugins.categories.${category}`) }}
                </li>
              </ul>
            </div>
          </div>
        </div>
      </v-container>
    </section>

    <section class="plugin-detail__sections section">
      <v-container>
        <div class="plugin-detail__grid">
          <article class="plugin-detail__panel">
            <p class="plugin-detail__panel-eyebrow">{{ t('plugins.useCasesTitle') }}</p>
            <h2 class="plugin-detail__panel-title">{{ t('plugins.useCasesTitle') }}</h2>
            <ul class="plugin-detail__list">
              <li v-for="item in plugin.useCases" :key="item" class="plugin-detail__list-item">
                {{ item }}
              </li>
            </ul>
          </article>

          <article class="plugin-detail__panel">
            <p class="plugin-detail__panel-eyebrow">{{ t('plugins.highlightsTitle') }}</p>
            <h2 class="plugin-detail__panel-title">{{ t('plugins.highlightsTitle') }}</h2>
            <ul class="plugin-detail__list">
              <li v-for="item in plugin.highlights" :key="item" class="plugin-detail__list-item">
                {{ item }}
              </li>
            </ul>
          </article>
        </div>
      </v-container>
    </section>
  </div>
</template>

<style scoped>
.plugin-detail {
  position: relative;
  min-height: 100vh;
}

.plugin-detail__hero {
  padding-top: 120px;
  padding-bottom: 16px;
}

.plugin-detail__hero-shell {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(300px, 0.75fr);
  gap: 28px;
  align-items: start;
}

.plugin-detail__hero-copy,
.plugin-detail__summary-card,
.plugin-detail__panel {
  border-radius: 28px;
  border: 1px solid color-mix(in srgb, var(--accent) 16%, rgba(255, 255, 255, 0.08));
  background:
    radial-gradient(
      circle at top right,
      color-mix(in srgb, var(--accent) 14%, transparent) 0%,
      transparent 36%
    ),
    rgba(10, 10, 15, 0.82);
  box-shadow: 0 24px 72px rgba(0, 0, 0, 0.26);
  backdrop-filter: blur(14px);
}

.plugin-detail__hero-copy {
  padding: 30px;
}

.plugin-detail__summary-card {
  padding: 24px;
}

.plugin-detail__panel-eyebrow,
.plugin-detail__summary-eyebrow,
.plugin-detail__summary-title {
  margin: 0;
  font-size: 0.72rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  font-family: 'JetBrains Mono', monospace;
}

.plugin-detail__panel-eyebrow {
  color: var(--accent);
}

.plugin-detail__summary-eyebrow,
.plugin-detail__summary-title {
  color: #7dd3fc;
}

.plugin-detail__headline {
  margin-top: 18px;
  display: flex;
  gap: 18px;
  align-items: flex-start;
}

.plugin-detail__logo-wrap {
  width: 72px;
  height: 72px;
  border-radius: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid color-mix(in srgb, var(--accent) 30%, rgba(255, 255, 255, 0.08));
  flex-shrink: 0;
}

.plugin-detail__logo-wrap--light {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(243, 246, 255, 0.96));
  border-color: rgba(255, 255, 255, 0.64);
  box-shadow:
    0 12px 28px rgba(0, 0, 0, 0.22),
    inset 0 1px 0 rgba(255, 255, 255, 0.88);
}

.plugin-detail__logo {
  width: 38px;
  height: 38px;
  object-fit: contain;
}

.plugin-detail__title {
  margin: 0 0 12px;
  font-size: clamp(2.2rem, 5vw, 4rem);
  line-height: 0.98;
  letter-spacing: -0.05em;
  color: #eff6ff;
}

.plugin-detail__tagline {
  margin: 0;
  color: #d9ecff;
  font-size: 1.05rem;
  line-height: 1.7;
  max-width: 680px;
}

.plugin-detail__chips {
  margin-top: 20px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.plugin-detail__status,
.plugin-detail__category {
  border-radius: 999px;
  padding: 9px 12px;
  font-size: 0.8rem;
  font-weight: 700;
}

.plugin-detail__status {
  background: rgba(57, 255, 20, 0.1);
  color: #39ff14;
  border: 1px solid rgba(57, 255, 20, 0.2);
}

.plugin-detail__category {
  background: rgba(255, 255, 255, 0.05);
  color: #dbe7ff;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.plugin-detail__summary {
  margin: 22px 0 0;
  color: #91a0bf;
  font-size: 1rem;
  line-height: 1.8;
  max-width: 720px;
}

.plugin-detail__actions {
  margin-top: 24px;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.plugin-detail__primary-cta {
  background: color-mix(in srgb, var(--accent) 88%, #0f172a) !important;
  color: #04131d !important;
  font-weight: 800 !important;
}

.plugin-detail__secondary-cta {
  border-color: rgba(125, 211, 252, 0.2) !important;
  color: #dffaff !important;
}

.plugin-detail__summary-block + .plugin-detail__summary-block {
  margin-top: 22px;
}

.plugin-detail__summary-title {
  margin-bottom: 12px;
}

.plugin-detail__badges {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.plugin-detail__sections {
  padding-top: 8px;
}

.plugin-detail__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 24px;
}

.plugin-detail__panel {
  padding: 26px;
}

.plugin-detail__panel-title {
  margin: 12px 0 0;
  color: #eff6ff;
  font-size: 1.5rem;
}

.plugin-detail__list {
  margin: 18px 0 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 14px;
}

.plugin-detail__list-item {
  position: relative;
  padding-left: 18px;
  color: #a9b7d7;
  line-height: 1.72;
}

.plugin-detail__list-item::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0.75em;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--accent);
  box-shadow: 0 0 18px color-mix(in srgb, var(--accent) 54%, transparent);
}

.v-theme--light .plugin-detail__hero-copy,
.v-theme--light .plugin-detail__summary-card,
.v-theme--light .plugin-detail__panel {
  background:
    radial-gradient(
      circle at top right,
      color-mix(in srgb, var(--accent) 10%, transparent) 0%,
      transparent 36%
    ),
    rgba(255, 255, 255, 0.92);
}

.v-theme--light .plugin-detail__title,
.v-theme--light .plugin-detail__panel-title,
.v-theme--light .plugin-detail__category,
.v-theme--light :deep(.agent-badge) {
  color: #0f172a;
}

.v-theme--light .plugin-detail__logo-wrap--light {
  background: #ffffff;
  border-color: rgba(148, 163, 184, 0.3);
  box-shadow:
    0 10px 26px rgba(15, 23, 42, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.92);
}

.v-theme--light .plugin-detail__tagline,
.v-theme--light .plugin-detail__summary,
.v-theme--light .plugin-detail__list-item {
  color: #475569;
}

@media (max-width: 960px) {
  .plugin-detail__hero {
    padding-top: 104px;
    padding-bottom: 8px;
  }

  .plugin-detail__hero-shell,
  .plugin-detail__grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .plugin-detail__hero-copy,
  .plugin-detail__summary-card,
  .plugin-detail__panel {
    padding: 20px;
    border-radius: 22px;
  }

  .plugin-detail__headline {
    gap: 14px;
  }

  .plugin-detail__logo-wrap {
    width: 58px;
    height: 58px;
    border-radius: 18px;
  }

  .plugin-detail__logo {
    width: 30px;
    height: 30px;
  }

  .plugin-detail__tagline,
  .plugin-detail__summary {
    font-size: 0.95rem;
    line-height: 1.66;
  }

  .plugin-detail__actions :deep(.v-btn) {
    width: 100%;
  }
}
</style>
