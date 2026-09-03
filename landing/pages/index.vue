<script setup lang="ts">
const registry = useRegistry();
const description =
  'Install, update, repair, and remove Agent Plugins 1.0 across supported AI agents with one CLI.';

useSeoMeta({
  title: 'Universal Agent Plugins',
  description,
  ogTitle: 'Universal Agent Plugins',
  ogDescription: description,
  ogType: 'website',
  twitterCard: 'summary',
});
</script>

<template>
  <div class="registry-surface unified-home">
    <PageBackground />
    <RegistryHero :registry="registry" />
    <RegistryDirectory :registry="registry" />
    <RegistryWhy />
    <RegistryFaq />
  </div>
</template>
