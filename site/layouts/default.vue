<script setup lang="ts">
const registry = useRegistry()
const { expired } = useDirectoryStatus(true)

function conciseDate(value: string): string {
  return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value))
}
</script>

<template>
  <div class="app-shell">
    <a class="skip-link" href="#main-content">Skip to content</a>
    <PageBackground />
    <AppHeader />
    <div v-if="registry.data_source === 'review_preview'" class="preview-banner" role="status">Pull request preview — unresolved review data is shown for review only. Production commands come from a published signed Directory snapshot.</div>
    <div v-else-if="registry.data_source === 'published_snapshot' && expired" class="directory-meta directory-meta--stale" role="alert">
      <strong>Plugin updates are temporarily paused.</strong> The catalog expired <time :datetime="registry.expires_at">{{ conciseDate(registry.expires_at!) }}</time>; browsing still works, but install commands are disabled until it refreshes.
    </div>
    <main id="main-content">
      <slot />
    </main>
    <AppFooter />
  </div>
</template>
