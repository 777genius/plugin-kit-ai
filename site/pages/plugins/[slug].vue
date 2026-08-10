<script setup lang="ts">
import { validationLabel } from '~/utils/registry'
const route = useRoute()
const registry = useRegistry()
const { pluginIcon, sourceUrl, repositoryUrl } = useSite()
const plugin = registry.plugins.find(item => item.name === route.params.slug)

if (!plugin) {
  throw createError({ statusCode: 404, statusMessage: 'Plugin not found' })
}

const canonical = `${useRuntimeConfig().public.siteUrl}/plugins/${plugin.name}`
useSeoMeta({
  title: plugin.name,
  description: plugin.description,
  ogTitle: `${plugin.name} · Universal Agent Plugins`,
  ogDescription: plugin.description,
  ogType: 'website',
})
useHead({ link: [{ rel: 'canonical', href: canonical }] })
</script>

<template>
  <div class="plugin-page container">
    <nav class="breadcrumbs" aria-label="Breadcrumb"><NuxtLink to="/plugins">Directory</NuxtLink><span aria-hidden="true">/</span><span aria-current="page">{{ plugin.name }}</span></nav>
    <div class="plugin-page__grid">
      <article class="plugin-profile">
        <div class="plugin-profile__heading">
          <span class="plugin-profile__icon"><img :src="pluginIcon(plugin)" alt="" width="54" height="54" /></span>
          <div><div class="plugin-profile__meta"><span class="source-pill">{{ plugin.built_in ? 'Built-in' : 'External' }}</span><span>v{{ plugin.version }}</span></div><h1>{{ plugin.name }}</h1></div>
        </div>
        <p class="plugin-profile__description">{{ plugin.description }}</p>
        <dl class="plugin-facts">
          <div><dt>Author</dt><dd>{{ plugin.author.name }}</dd></div>
          <div><dt>License</dt><dd>{{ plugin.license || 'Not specified' }}</dd></div>
          <div><dt>Install source</dt><dd><code>{{ plugin.install_source }}</code></dd></div>
          <div><dt>Repository</dt><dd><a :href="sourceUrl(plugin)" target="_blank" rel="noreferrer">View source <span aria-hidden="true">↗</span></a></dd></div>
        </dl>
        <div class="plugin-profile__section"><h2>Components</h2><ul class="badge-list"><li v-for="component in plugin.components" :key="component">{{ component }}</li></ul></div>
        <div v-if="plugin.categories.length" class="plugin-profile__section"><h2>Categories</h2><ul class="tag-list"><li v-for="category in plugin.categories" :key="category">{{ category }}</li></ul></div>
        <div class="status-card">
          <span class="validation-badge"><span>✓</span> {{ validationLabel(plugin.validation) }}</span>
          <p v-if="plugin.validation.runtime_evidence.length"><strong>Runtime evidence:</strong> {{ plugin.validation.runtime_evidence.join(', ') }}.</p>
          <p v-else>No runtime evidence is recorded for this exact revision. Schema validation covers package structure only; it is not an endorsement or a runtime safety claim.</p>
          <a :href="`${repositoryUrl}/blob/main/docs/VERIFICATION.md`" target="_blank" rel="noreferrer">Read verification evidence →</a>
        </div>
      </article>
      <InstallPanel :plugin="plugin" />
    </div>
  </div>
</template>
