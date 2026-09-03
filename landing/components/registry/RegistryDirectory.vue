<script setup lang="ts">
import type { RegistryIndex } from '~/types/registry';

const props = defineProps<{ registry: RegistryIndex }>();
const reviewedCount = computed(
  () =>
    props.registry.plugins.filter((plugin) => plugin.trust_state !== 'conformant_unreviewed')
      .length,
);
const discoveryCount = computed(() => props.registry.plugins.length - reviewedCount.value);
const intro = computed(() => {
  const reviewed = `${reviewedCount.value} reviewed plugins`;
  const discovered = discoveryCount.value ? ` plus ${discoveryCount.value} community packages` : '';
  return `${reviewed}${discovered}. Search, choose your agents, and copy one command.`;
});
</script>

<template>
  <div id="plugins" class="container catalog-wrap">
    <PluginCatalog :plugins="registry.plugins" heading="Find your plugin" :intro="intro" />
  </div>
</template>
