<script setup lang="ts">
import { availableFilters, filterPlugins } from '~/utils/filter';
import type { RegistryPlugin } from '~/types/registry';

const props = withDefaults(
  defineProps<{ plugins: RegistryPlugin[]; heading?: string; intro?: string }>(),
  {
    heading: 'Explore plugins',
    intro: 'Search by capability, component, or source.',
  },
);
const { asset, repositoryUrl } = useSite();
const query = ref('');
const category = ref('all');
const component = ref('all');
const source = ref('all');
const trust = ref('all');
const client = ref('all');
const authentication = ref('all');
const owner = ref('all');
const pageSize = 48;
const displayLimit = ref(pageSize);
const discovery = useDiscoveryStatus();
const filters = computed(() => availableFilters(props.plugins));
type FilterOption = { value: string; label: string };
let previousCategoryOptions: FilterOption[] = [];
let previousComponentOptions: FilterOption[] = [];
function stableOptions(next: FilterOption[], previous: FilterOption[]) {
  return next.length === previous.length &&
    next.every(
      (item, index) =>
        item.value === previous[index]?.value && item.label === previous[index]?.label,
    )
    ? previous
    : next;
}
const stableCategoryOptions = computed(() => {
  const next = [
    { value: 'all', label: 'All categories' },
    ...filters.value.categories.map((item) => ({ value: item, label: item })),
  ];
  previousCategoryOptions = stableOptions(next, previousCategoryOptions);
  return previousCategoryOptions;
});
const stableComponentOptions = computed(() => {
  const next = [
    { value: 'all', label: 'All components' },
    ...filters.value.components.map((item) => ({ value: item, label: item })),
  ];
  previousComponentOptions = stableOptions(next, previousComponentOptions);
  return previousComponentOptions;
});
const sourceOptions = [
  { value: 'all', label: 'All sources' },
  { value: 'upstream', label: 'Upstream packages' },
  { value: 'community_bridge', label: 'Community bridges' },
  { value: 'community', label: 'Community packages' },
  { value: 'direct', label: 'Direct sources' },
];
const trustOptions = [
  { value: 'all', label: 'All trust levels' },
  { value: 'reviewed', label: 'Reviewed plugins' },
  { value: 'conformant_unreviewed', label: 'Community discovery' },
];
const clientOptions = [
  { value: 'all', label: 'All agents' },
  ...clients.map((item) => ({
    value: item.id,
    label: item.name,
    icon: asset(`client-icons/${item.icon}`),
  })),
];
const authenticationOptions = [
  { value: 'all', label: 'All authentication' },
  { value: 'none', label: 'Works without sign-in' },
  { value: 'required_or_unknown', label: 'May require sign-in' },
];
const ownerOptions = computed(() => [
  { value: 'all', label: 'All owners' },
  ...filters.value.owners.map((item) => ({ value: item, label: item })),
]);
const visible = computed(() =>
  filterPlugins(props.plugins, {
    query: query.value,
    category: category.value === 'all' ? '' : category.value,
    component:
      component.value === 'all'
        ? undefined
        : (component.value as RegistryPlugin['components'][number]),
    source: source.value as 'all' | 'upstream' | 'community_bridge' | 'community' | 'direct',
    trust: trust.value as 'all' | 'reviewed' | 'conformant_unreviewed',
    client: client.value as 'all' | RegistryPlugin['client_support']['clients'][number],
    authentication: authentication.value as 'all' | 'none' | 'required_or_unknown',
    owner: owner.value === 'all' ? '' : owner.value,
  }),
);
const displayed = computed(() => visible.value.slice(0, displayLimit.value));
const remaining = computed(() => Math.max(0, visible.value.length - displayed.value.length));
watch([query, category, component, source, trust, client, authentication, owner], () => {
  displayLimit.value = pageSize;
});
</script>

<template>
  <section class="catalog" aria-labelledby="catalog-title">
    <div class="section-heading">
      <p class="eyebrow">Plugin directory</p>
      <div class="catalog-heading-row">
        <h2 id="catalog-title">{{ heading }}</h2>
        <a
          class="catalog-add-button"
          :href="`${repositoryUrl}/blob/main/registry/README.md#submit-an-external-package`"
          target="_blank"
          rel="noreferrer"
          aria-label="Add a plugin"
          aria-describedby="catalog-add-tooltip"
        >
          <span aria-hidden="true">＋</span>
          <span id="catalog-add-tooltip" class="catalog-add-button__tooltip" role="tooltip"
            >Add a plugin</span
          >
        </a>
      </div>
      <p>{{ intro }}</p>
    </div>
    <div class="catalog-controls" role="search" aria-label="Filter plugins">
      <label class="search-field">
        <span class="sr-only">Search plugins</span>
        <svg class="search-field__icon" aria-hidden="true" viewBox="0 0 24 24" fill="none">
          <circle cx="11" cy="11" r="6.5" />
          <path d="m16 16 4 4" />
        </svg>
        <input v-model="query" type="search" placeholder="Search by name, author, or capability…" >
      </label>
      <AppCombobox
        v-model="category"
        leading-icon="category"
        label="Filter by category"
        search-placeholder="Search categories…"
        :options="stableCategoryOptions"
      />
      <AppSelect
        v-model="component"
        leading-icon="component"
        label="Filter by component"
        :options="stableComponentOptions"
      />
      <AppSelect
        v-model="source"
        leading-icon="source"
        label="Filter by source"
        :options="sourceOptions"
      />
      <AppSelect
        v-model="trust"
        leading-icon="trust"
        label="Filter by trust level"
        :options="trustOptions"
      />
      <AppSelect
        v-model="client"
        leading-icon="agent"
        label="Filter by agent"
        :options="clientOptions"
      />
      <AppSelect
        v-model="authentication"
        leading-icon="authentication"
        label="Filter by authentication"
        :options="authenticationOptions"
      />
      <AppCombobox
        v-model="owner"
        leading-icon="owner"
        label="Filter by owner"
        search-placeholder="Search owners…"
        :options="ownerOptions"
      />
    </div>
    <div class="catalog-meta">
      <div>
        <div class="catalog-count" aria-live="polite">
          Showing {{ displayed.length }} of {{ visible.length }} matching plugins ·
          {{ plugins.length }} total
        </div>
        <p
          v-if="['loading', 'stale', 'unavailable'].includes(discovery.state)"
          class="discovery-status"
          :class="`discovery-status--${discovery.state}`"
        >
          <template v-if="discovery.state === 'loading'"
            >Finding more community plugins on GitHub…</template
          >
          <template v-else-if="discovery.state === 'stale'"
            >Community results are refreshing. Reviewed plugins remain available.</template
          >
          <template v-else-if="discovery.state === 'unavailable'"
            >Community results are temporarily unavailable. Reviewed plugins remain
            available.</template
          >
        </p>
      </div>
    </div>
    <div v-if="visible.length" class="plugin-grid">
      <RegistryPluginCard
        v-for="plugin in displayed"
        :key="plugin.install_source"
        :plugin="plugin"
      />
    </div>
    <div v-else class="empty-state">
      <h3>No matching plugins</h3>
      <p>Try a broader search or clear one of the filters.</p>
      <button
        class="button button--secondary"
        type="button"
        @click="
          query = '';
          category = 'all';
          component = 'all';
          source = 'all';
          trust = 'all';
          client = 'all';
          authentication = 'all';
          owner = 'all';
        "
      >
        Clear filters
      </button>
    </div>
    <div v-if="remaining" class="catalog-more">
      <button class="button button--secondary" type="button" @click="displayLimit += pageSize">
        Show {{ Math.min(pageSize, remaining) }} more <span aria-hidden="true">↓</span>
      </button>
      <span>{{ remaining }} matching plugins remaining</span>
    </div>
    <div class="catalog-end-submit">
      <a
        class="button button--secondary"
        :href="`${repositoryUrl}/blob/main/registry/README.md#submit-an-external-package`"
        target="_blank"
        rel="noreferrer"
        >Add a plugin by pull request <span aria-hidden="true">↗</span></a
      >
    </div>
  </section>
</template>
