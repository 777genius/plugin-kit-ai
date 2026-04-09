<script setup lang="ts">
const { content } = useLandingContent();
const { t } = useI18n();
const localePath = useLocalePath();
const searchQuery = ref('');
const selectedPluginType = ref('all');
const selectedCategory = ref('all');

usePageSeo('meta.pluginsTitle', 'meta.pluginsDescription');

const normalizedQuery = computed(() => searchQuery.value.trim().toLowerCase());

const categoryOptions = computed(() => {
  const categories = new Set<string>();

  for (const plugin of content.value.plugins) {
    for (const category of plugin.categories) {
      categories.add(category);
    }
  }

  return ['all', ...categories];
});

const pluginTypeOptions = computed(() => {
  const types = new Set<string>();
  for (const plugin of content.value.plugins) {
    types.add(plugin.pluginType);
  }
  return ['all', ...types];
});

const searchFilteredPlugins = computed(() => {
  const query = normalizedQuery.value;
  if (!query) return content.value.plugins;

  return content.value.plugins.filter((plugin) => {
    const haystack = [
      plugin.title,
      plugin.tagline,
      plugin.description,
      plugin.eyebrow,
      plugin.status,
      t(`plugins.types.${plugin.pluginType}`),
      ...plugin.badges,
      ...plugin.categories.map((category) => t(`plugins.categories.${category}`)),
    ]
      .join(' ')
      .toLowerCase();

    return haystack.includes(query);
  });
});

const filteredPlugins = computed(() =>
  searchFilteredPlugins.value.filter((plugin) =>
    (selectedPluginType.value === 'all' ? true : plugin.pluginType === selectedPluginType.value) &&
    (selectedCategory.value === 'all' ? true : plugin.categories.includes(selectedCategory.value)),
  ),
);

const hasActiveSearch = computed(() => normalizedQuery.value.length > 0);
const hasActiveFilters = computed(
  () => hasActiveSearch.value || selectedCategory.value !== 'all' || selectedPluginType.value !== 'all',
);
const showInlineClearButton = computed(
  () => hasActiveSearch.value && filteredPlugins.value.length > 0,
);

function clearSearch() {
  searchQuery.value = '';
}

function resetFilters() {
  searchQuery.value = '';
  selectedPluginType.value = 'all';
  selectedCategory.value = 'all';
}

function pluginDetailPath(slug: string) {
  return localePath(`/plugins/${slug}`);
}
</script>

<template>
  <div class="plugins-page">
    <PageBackground />

    <section class="plugins-page__hero section">
      <v-container>
        <div class="plugins-page__hero-inner">
          <p class="plugins-page__eyebrow">{{ t('plugins.catalogEyebrow') }}</p>
          <h1 class="plugins-page__title">{{ t('plugins.catalogTitle') }}</h1>
          <p class="plugins-page__subtitle">{{ t('plugins.catalogSubtitle') }}</p>

          <div class="plugins-page__search-shell">
            <label class="plugins-page__search-label" for="plugins-search">
              {{ t('plugins.searchLabel') }}
            </label>
            <div class="plugins-page__search-row">
              <input
                id="plugins-search"
                v-model="searchQuery"
                type="search"
                :placeholder="t('plugins.searchPlaceholder')"
                class="plugins-page__search-input"
              >
              <button
                v-if="showInlineClearButton"
                type="button"
                class="plugins-page__clear-btn"
                @click="clearSearch"
              >
                {{ t('plugins.clearSearch') }}
              </button>
            </div>

            <div class="plugins-page__filters">
              <div class="plugins-page__filters-label">
                {{ t('plugins.typeFilterLabel') }}
              </div>
              <div
                class="plugins-page__chips"
                role="tablist"
                :aria-label="t('plugins.typeFilterLabel')"
              >
                <button
                  v-for="pluginType in pluginTypeOptions"
                  :key="pluginType"
                  type="button"
                  class="plugins-page__chip"
                  :class="{ 'is-active': selectedPluginType === pluginType }"
                  @click="selectedPluginType = pluginType"
                >
                  {{
                    pluginType === 'all'
                      ? t('plugins.allTypes')
                      : t(`plugins.types.${pluginType}`)
                  }}
                </button>
              </div>
            </div>

            <div class="plugins-page__filters">
              <div class="plugins-page__filters-label">
                {{ t('plugins.filterLabel') }}
              </div>
              <div
                class="plugins-page__chips"
                role="tablist"
                :aria-label="t('plugins.filterLabel')"
              >
                <button
                  v-for="category in categoryOptions"
                  :key="category"
                  type="button"
                  class="plugins-page__chip"
                  :class="{ 'is-active': selectedCategory === category }"
                  @click="selectedCategory = category"
                >
                  {{
                    category === 'all'
                      ? t('plugins.allCategories')
                      : t(`plugins.categories.${category}`)
                  }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </v-container>
    </section>

    <section class="plugins-page__grid section">
      <v-container>
        <div v-if="filteredPlugins.length" class="plugins-page__cards">
          <PluginCard
            v-for="plugin in filteredPlugins"
            :key="plugin.id"
            :plugin="plugin"
            :highlight-query="searchQuery"
            :supports-label="t('plugins.supports')"
            :open-label="t('plugins.viewDetails')"
            :to="pluginDetailPath(plugin.slug)"
          />
        </div>

        <div v-else class="plugins-page__empty">
          <div class="plugins-page__empty-icon">0</div>
          <h2 class="plugins-page__empty-title">{{ t('plugins.emptyTitle') }}</h2>
          <p class="plugins-page__empty-text">{{ t('plugins.emptyText') }}</p>
          <button type="button" class="plugins-page__empty-action" @click="resetFilters">
            {{ hasActiveFilters ? t('plugins.clearSearch') : t('plugins.backToCatalog') }}
          </button>
        </div>
      </v-container>
    </section>
  </div>
</template>

<style scoped>
.plugins-page {
  position: relative;
  min-height: 100vh;
}

.plugins-page__hero {
  padding-top: 56px;
  padding-bottom: 20px;
}

.plugins-page__hero-inner {
  max-width: 920px;
  margin: 0 auto;
}

.plugins-page__eyebrow {
  margin: 0 0 14px;
  color: #39ff14;
  text-transform: uppercase;
  letter-spacing: 0.16em;
  font-size: 0.74rem;
  font-family: 'JetBrains Mono', monospace;
}

.plugins-page__title {
  margin: 0 0 18px;
  font-size: clamp(2.4rem, 5vw, 4.2rem);
  line-height: 1.02;
  letter-spacing: -0.05em;
  font-weight: 800;
  color: #eff6ff;
}

.plugins-page__subtitle {
  margin: 0 0 24px;
  max-width: 720px;
  color: #91a0bf;
  font-size: 1.05rem;
  line-height: 1.72;
}

.plugins-page__search-shell {
  border-radius: 24px;
  padding: 18px;
  background: rgba(10, 10, 15, 0.78);
  border: 1px solid rgba(0, 240, 255, 0.12);
  box-shadow: 0 18px 64px rgba(0, 0, 0, 0.24);
  backdrop-filter: blur(14px);
}

.plugins-page__search-label,
.plugins-page__filters-label {
  display: block;
  font-size: 0.75rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #7dd3fc;
  font-family: 'JetBrains Mono', monospace;
}

.plugins-page__search-label {
  margin-bottom: 10px;
}

.plugins-page__search-row {
  display: flex;
  gap: 12px;
  align-items: center;
}

.plugins-page__search-input {
  width: 100%;
  min-width: 0;
  border: 1px solid rgba(148, 163, 184, 0.2);
  background: rgba(255, 255, 255, 0.03);
  color: #eff6ff;
  border-radius: 16px;
  padding: 16px 18px;
  font-size: 1rem;
  outline: none;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease,
    background 0.2s ease;
}

.plugins-page__search-input:focus {
  border-color: rgba(0, 240, 255, 0.42);
  box-shadow: 0 0 0 4px rgba(0, 240, 255, 0.09);
  background: rgba(255, 255, 255, 0.05);
}

.plugins-page__search-input::placeholder {
  color: #718096;
}

.plugins-page__filters {
  margin-top: 16px;
}

.plugins-page__filters-label {
  margin-bottom: 10px;
}

.plugins-page__chips {
  display: flex;
  gap: 10px;
  overflow-x: auto;
  padding-bottom: 4px;
  scrollbar-width: none;
}

.plugins-page__chips::-webkit-scrollbar {
  display: none;
}

.plugins-page__chip {
  flex-shrink: 0;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.04);
  color: #dbe7ff;
  padding: 10px 14px;
  font-size: 0.82rem;
  font-weight: 700;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background 0.2s ease,
    color 0.2s ease,
    transform 0.2s ease;
}

.plugins-page__chip:hover {
  transform: translateY(-1px);
  border-color: rgba(0, 240, 255, 0.3);
}

.plugins-page__chip.is-active {
  border-color: rgba(0, 240, 255, 0.42);
  background: rgba(0, 240, 255, 0.12);
  color: #00f0ff;
}

.plugins-page__clear-btn,
.plugins-page__empty-action {
  flex-shrink: 0;
  border: 1px solid rgba(0, 240, 255, 0.2);
  background: rgba(0, 240, 255, 0.08);
  color: #00f0ff;
  border-radius: 14px;
  padding: 13px 16px;
  font-size: 0.9rem;
  font-weight: 700;
  cursor: pointer;
  transition:
    transform 0.2s ease,
    border-color 0.2s ease,
    background 0.2s ease;
  white-space: nowrap;
}

.plugins-page__clear-btn:hover,
.plugins-page__empty-action:hover {
  transform: translateY(-1px);
  border-color: rgba(0, 240, 255, 0.38);
  background: rgba(0, 240, 255, 0.14);
}

.plugins-page__grid {
  padding-top: 8px;
}

.plugins-page__cards {
  display: grid;
  gap: 24px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.plugins-page__empty {
  max-width: 560px;
  margin: 20px auto 0;
  text-align: center;
  padding: 40px 28px;
  border-radius: 28px;
  border: 1px solid rgba(0, 240, 255, 0.12);
  background: rgba(10, 10, 15, 0.72);
  box-shadow: 0 18px 64px rgba(0, 0, 0, 0.2);
}

.plugins-page__empty-icon {
  width: 72px;
  height: 72px;
  border-radius: 20px;
  margin: 0 auto 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(0, 240, 255, 0.18), rgba(57, 255, 20, 0.18));
  color: #dffaff;
  font-size: 1.4rem;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.plugins-page__empty-title {
  margin: 0 0 10px;
  color: #eff6ff;
  font-size: 1.55rem;
}

.plugins-page__empty-text {
  margin: 0 0 22px;
  color: #91a0bf;
  line-height: 1.7;
}

.v-theme--light .plugins-page__title {
  color: #0f172a;
}

.v-theme--light .plugins-page__subtitle,
.v-theme--light .plugins-page__empty-text {
  color: #475569;
}

.v-theme--light .plugins-page__search-shell,
.v-theme--light .plugins-page__empty {
  background: rgba(255, 255, 255, 0.9);
}

.v-theme--light .plugins-page__search-input {
  background: rgba(15, 23, 42, 0.03);
  color: #0f172a;
}

.v-theme--light .plugins-page__search-input::placeholder {
  color: #94a3b8;
}

.v-theme--light .plugins-page__empty-title,
.v-theme--light .plugins-page__chip {
  color: #0f172a;
}

@media (max-width: 1264px) {
  .plugins-page__cards {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .plugins-page__hero {
    padding-top: 28px;
    padding-bottom: 8px;
  }

  .plugins-page__subtitle {
    margin-bottom: 18px;
    font-size: 0.96rem;
    line-height: 1.64;
  }

  .plugins-page__search-shell {
    padding: 14px;
    border-radius: 20px;
  }

  .plugins-page__search-row {
    flex-direction: column;
    align-items: stretch;
  }

  .plugins-page__clear-btn {
    width: 100%;
  }

  .plugins-page__chips {
    margin-inline: -4px;
    padding-inline: 4px 4px;
  }

  .plugins-page__cards {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 600px) {
  .plugins-page__hero {
    padding-top: 12px;
  }
}
</style>
