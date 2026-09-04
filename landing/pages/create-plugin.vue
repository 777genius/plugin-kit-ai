<script setup lang="ts">
// Keep the legacy authoring page out of search until its plugin.yaml workflow
// is replaced by the approved plugin.json-first Agent Plugins 1.0 flow.
usePageSeo(
  'Create an Agent Plugin 1.0 Package | Universal Agent Plugins',
  'A future guide for creating portable Agent Plugins 1.0 packages for multiple AI agents.',
  {
    translate: false,
    robots: 'noindex, follow',
  },
);
useTrackSections();
const { containerRef } = useParallaxSections();
</script>

<template>
  <div ref="containerRef" class="page">
    <PageBackground />
    <HeroSection />
    <SectionDivider />
    <LazyFeaturesSection />
    <SectionDivider :flip="true" />
    <LazyScreenshotsSection />
    <SectionDivider />
    <LazyCustomLogicSection />
    <SectionDivider :flip="true" />
    <LazyDownloadSection />
    <SectionDivider :flip="true" />
    <LazyComparisonSection />
    <SectionDivider />
    <LazyFAQSection />
  </div>
</template>
