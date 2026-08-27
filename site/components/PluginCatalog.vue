<script setup lang="ts">
import { availableFilters, filterPlugins } from '~/utils/filter'
import type { RegistryPlugin } from '~/types/registry'

const props = withDefaults(defineProps<{ plugins: RegistryPlugin[], heading?: string, intro?: string }>(), {
  heading: 'Explore plugins',
  intro: 'Search by capability, component, or source.',
})
const { repositoryUrl } = useSite()
const query = ref('')
const category = ref('all')
const component = ref('all')
const source = ref('all')
const trust = ref('all')
const client = ref('all')
const authentication = ref('all')
const owner = ref('all')
const pageSize = 48
const displayLimit = ref(pageSize)
const discovery = useDiscoveryStatus()
const filters = computed(() => availableFilters(props.plugins))
type FilterOption = { value: string, label: string }
let previousCategoryOptions: FilterOption[] = []
let previousComponentOptions: FilterOption[] = []
function stableOptions(next: FilterOption[], previous: FilterOption[]) {
  return next.length === previous.length && next.every((item, index) => item.value === previous[index]?.value && item.label === previous[index]?.label)
    ? previous
    : next
}
const stableCategoryOptions = computed(() => {
  const next = [{ value: 'all', label: 'All categories' }, ...filters.value.categories.map(item => ({ value: item, label: item }))]
  previousCategoryOptions = stableOptions(next, previousCategoryOptions)
  return previousCategoryOptions
})
const stableComponentOptions = computed(() => {
  const next = [{ value: 'all', label: 'All components' }, ...filters.value.components.map(item => ({ value: item, label: item }))]
  previousComponentOptions = stableOptions(next, previousComponentOptions)
  return previousComponentOptions
})
const sourceOptions = [
  { value: 'all', label: 'All sources' },
  { value: 'upstream', label: 'Upstream packages' },
  { value: 'community_bridge', label: 'Community bridges' },
  { value: 'community', label: 'Community packages' },
  { value: 'direct', label: 'Direct sources' },
]
const trustOptions = [
  { value: 'all', label: 'All trust levels' },
  { value: 'reviewed', label: 'Reviewed Directory' },
  { value: 'conformant_unreviewed', label: 'Schema conformant · unreviewed' },
]
const clientOptions = [
  { value: 'all', label: 'All agents' },
  ...clients.map(item => ({ value: item.id, label: item.name })),
]
const authenticationOptions = [
  { value: 'all', label: 'All authentication' },
  { value: 'none', label: 'No account required' },
  { value: 'required_or_unknown', label: 'Auth required or unknown' },
]
const ownerOptions = computed(() => [
  { value: 'all', label: 'All owners' },
  ...filters.value.owners.map(item => ({ value: item, label: item })),
])
const visible = computed(() => filterPlugins(props.plugins, {
  query: query.value,
  category: category.value === 'all' ? '' : category.value,
  component: component.value === 'all' ? undefined : component.value as RegistryPlugin['components'][number],
  source: source.value as 'all' | 'upstream' | 'community_bridge' | 'community' | 'direct',
  trust: trust.value as 'all' | 'reviewed' | 'conformant_unreviewed',
  client: client.value as 'all' | RegistryPlugin['client_support']['clients'][number],
  authentication: authentication.value as 'all' | 'none' | 'required_or_unknown',
  owner: owner.value === 'all' ? '' : owner.value,
}))
const displayed = computed(() => visible.value.slice(0, displayLimit.value))
const remaining = computed(() => Math.max(0, visible.value.length - displayed.value.length))
watch([query, category, component, source, trust, client, authentication, owner], () => {
  displayLimit.value = pageSize
})
</script>

<template>
  <section class="catalog" aria-labelledby="catalog-title">
    <div class="section-heading">
      <p class="eyebrow">Plugin directory</p>
      <h2 id="catalog-title">{{ heading }}</h2>
      <p>{{ intro }}</p>
    </div>
    <div class="catalog-controls" role="search" aria-label="Filter plugins">
      <label class="search-field">
        <span class="sr-only">Search plugins</span>
        <svg class="search-field__icon" aria-hidden="true" viewBox="0 0 24 24" fill="none">
          <circle cx="11" cy="11" r="6.5" />
          <path d="m16 16 4 4" />
        </svg>
        <input v-model="query" type="search" placeholder="Search by name, author, or capability…" />
      </label>
      <AppCombobox v-model="category" label="Filter by category" search-placeholder="Search categories…" :options="stableCategoryOptions" />
      <AppSelect v-model="component" label="Filter by component" :options="stableComponentOptions" />
      <AppSelect v-model="source" label="Filter by source" :options="sourceOptions" />
      <AppSelect v-model="trust" label="Filter by trust level" :options="trustOptions" />
      <AppSelect v-model="client" label="Filter by agent" :options="clientOptions" />
      <AppSelect v-model="authentication" label="Filter by authentication" :options="authenticationOptions" />
      <AppCombobox v-model="owner" label="Filter by owner" search-placeholder="Search owners…" :options="ownerOptions" />
    </div>
    <div class="catalog-meta">
      <div>
        <div class="catalog-count" aria-live="polite">Showing {{ displayed.length }} of {{ visible.length }} matching plugins · {{ plugins.length }} total</div>
        <p class="discovery-status" :class="`discovery-status--${discovery.state}`">
          <template v-if="discovery.state === 'loading'">Loading signed public Discovery Index…</template>
          <template v-else-if="discovery.state === 'current'">{{ discovery.count }} unreviewed packages from signed index {{ discovery.sequence }} · updated {{ discovery.generatedAt }}</template>
          <template v-else-if="discovery.state === 'cached'">{{ discovery.count }} unreviewed packages from last-known-good signed index {{ discovery.sequence }}</template>
          <template v-else-if="discovery.state === 'stale'">Public Discovery Index is stale. Reviewed Directory remains available.</template>
          <template v-else-if="discovery.state === 'unavailable'">Public Discovery Index is unavailable. Reviewed Directory remains available.</template>
          <template v-else>Reviewed Directory ready.</template>
        </p>
      </div>
      <a class="button button--secondary catalog-submit" :href="`${repositoryUrl}/blob/main/registry/README.md#submit-an-external-package`" target="_blank" rel="noreferrer">
        <span aria-hidden="true">＋</span> Add a plugin
      </a>
    </div>
    <div v-if="visible.length" class="plugin-grid">
      <PluginCard v-for="plugin in displayed" :key="plugin.install_source" :plugin="plugin" />
    </div>
    <div v-else class="empty-state">
      <h3>No matching plugins</h3>
      <p>Try a broader search or clear one of the filters.</p>
      <button class="button button--secondary" type="button" @click="query = ''; category = 'all'; component = 'all'; source = 'all'; trust = 'all'; client = 'all'; authentication = 'all'; owner = 'all'">Clear filters</button>
    </div>
    <div v-if="remaining" class="catalog-more">
      <button class="button button--secondary" type="button" @click="displayLimit += pageSize">
        Show {{ Math.min(pageSize, remaining) }} more <span aria-hidden="true">↓</span>
      </button>
      <span>{{ remaining }} matching plugins remaining</span>
    </div>
    <div class="catalog-end-submit"><a class="button button--secondary" :href="`${repositoryUrl}/blob/main/registry/README.md#submit-an-external-package`" target="_blank" rel="noreferrer">Add a plugin by pull request <span aria-hidden="true">↗</span></a></div>
  </section>
</template>
