<script setup lang="ts">
const registry = useRegistry();
const description = 'Browse reviewed and automatically discovered Agent Plugins 1.0 packages.';

useSeoMeta({
  title: 'Plugin directory · Universal Agent Plugins',
  description,
  ogTitle: 'Plugin directory · Universal Agent Plugins',
  ogDescription: description,
});
</script>

<template>
  <div class="registry-surface directory-page">
    <PageBackground />
    <div class="container">
      <div class="page-intro">
        <p class="eyebrow">Plugin directory</p>
        <h1>Find a plugin.<br ><em>Use it everywhere.</em></h1>
        <p>
          Search reviewed packages and community plugins discovered from public GitHub repositories.
        </p>
      </div>
      <PluginCatalog
        :plugins="registry.plugins"
        heading="Explore plugins"
        intro="Filter by capability, source, authentication, or supported agent."
      />
    </div>
  </div>
</template>
