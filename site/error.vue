<script setup lang="ts">
defineProps<{ error: { statusCode?: number, statusMessage?: string } }>()
</script>

<template>
  <NuxtLayout>
    <section class="error-page container">
      <p class="eyebrow">{{ error.statusCode || 500 }}</p>
      <h1>{{ error.statusMessage || 'Something went wrong' }}</h1>
      <p>The page may have moved or the generated directory does not contain this entry.</p>
      <NuxtLink class="button button--primary" to="/plugins">Browse plugins</NuxtLink>
    </section>
  </NuxtLayout>
</template>
