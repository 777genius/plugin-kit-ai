<script setup lang="ts">
const registry = useRegistry()
const description = 'Search the generated Universal Agent Plugins directory by name, category, component, and source.'
useSeoMeta({ title: 'Plugin directory', description, ogTitle: 'Plugin directory · Universal Agent Plugins', ogDescription: description })
useHead({ link: [{ rel: 'canonical', href: `${useRuntimeConfig().public.siteUrl}/plugins` }] })
</script>

<template>
  <div class="directory-page container">
    <div class="page-intro">
      <p class="eyebrow">Generated registry · {{ registry.plugins.length }} entries</p>
      <h1>Find the right ability for your agent.</h1>
      <p>Every entry exposes its source, package components, and validation status. External sources are accepted only with a full immutable commit pin.</p>
    </div>
    <PluginCatalog :plugins="registry.plugins" />
  </div>
</template>
