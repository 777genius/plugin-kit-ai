<script setup lang="ts">
const registry = useRegistry()
const reviewedCount = computed(() => registry.plugins.filter(plugin => plugin.trust_state !== 'conformant_unreviewed').length)
const discoveryCount = computed(() => registry.plugins.length - reviewedCount.value)
const description = 'Browse Agent Plugins 1.0 by capability, agent support, component, and source.'
useSeoMeta({ title: 'Plugin directory', description, ogTitle: 'Plugin directory · Universal Agent Plugins', ogDescription: description })
useHead({ link: [{ rel: 'canonical', href: `${useRuntimeConfig().public.siteUrl}/plugins` }] })
</script>

<template>
  <div class="directory-page container">
    <div class="page-intro">
      <p class="eyebrow">{{ reviewedCount }} reviewed<template v-if="discoveryCount"> · {{ discoveryCount }} discovered</template> · Open submissions</p>
      <h1>Find the right ability for your agent.</h1>
      <p>Browse by capability, agent support, and validation status. Community packages stay linked to reviewable GitHub source.</p>
    </div>
    <PluginCatalog :plugins="registry.plugins" />
  </div>
</template>
