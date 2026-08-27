<script setup lang="ts">
const route = useRoute()
const event = useRequestEvent()
if (event) setResponseStatus(event, 404)

const title = 'Page not found'
useSeoMeta({ title, robots: 'noindex, nofollow' })
useHead({ link: [{ rel: 'canonical', href: `${useRuntimeConfig().public.siteUrl}${route.path}` }] })
</script>

<template>
  <section class="error-page container">
    <p class="eyebrow">404</p>
    <h1>Page not found</h1>
    <p>The page may have moved or the generated directory does not contain this entry.</p>
    <NuxtLink class="button button--primary" to="/plugins">Browse plugins</NuxtLink>
  </section>
</template>
