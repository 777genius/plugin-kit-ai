<!--
  Card composition adapted from plugin-kit-ai landing/components/plugins/PluginCard.vue (MIT).
  Content model and implementation are new for Universal Agent Plugins.
-->
<script setup lang="ts">
import type { RegistryPlugin } from '~/types/registry'
import { validationLabel } from '~/utils/registry'

defineProps<{ plugin: RegistryPlugin }>()
const { pluginIcon, sourceUrl } = useSite()
</script>

<template>
  <article class="plugin-card">
    <NuxtLink class="plugin-card__main-link" :to="`/plugins/${plugin.name}`" :aria-label="`View ${plugin.name}`" />
    <div class="plugin-card__top">
      <span class="plugin-card__icon"><img :src="pluginIcon(plugin)" alt="" width="32" height="32" loading="lazy" /></span>
      <span class="source-pill">{{ plugin.built_in ? 'Built-in' : 'External' }}</span>
    </div>
    <h3>{{ plugin.name }}</h3>
    <p class="plugin-card__description">{{ plugin.description }}</p>
    <p class="plugin-card__author">By {{ plugin.author.name }} · <a :href="sourceUrl(plugin)" target="_blank" rel="noreferrer">source <span class="sr-only">for {{ plugin.name }}</span></a></p>
    <div class="plugin-card__bottom">
      <ul class="badge-list" aria-label="Plugin components">
        <li v-for="component in plugin.components" :key="component">{{ component }}</li>
      </ul>
      <span class="validation-badge">
        <span aria-hidden="true">✓</span> {{ validationLabel(plugin.validation) }}
      </span>
    </div>
  </article>
</template>
