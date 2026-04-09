<script setup lang="ts">
usePageSeo("meta.homeTitle", "meta.homeDescription");
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
