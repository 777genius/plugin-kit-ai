<script setup lang="ts">
import type { RegistryPlugin } from '~/types/registry';
import { securityAssessmentLabel, securityAssessmentTooltip } from '~/utils/securityPresentation';

const props = defineProps<{
  plugin: RegistryPlugin;
  detailsTo: string | { path: string; query?: Record<string, string>; hash?: string };
}>();
const assessment = computed(() => props.plugin.security!);
const label = computed(() => securityAssessmentLabel(assessment.value));
const tooltip = computed(() => securityAssessmentTooltip(props.plugin));
</script>

<template>
  <NuxtLink
    class="plugin-card__security"
    :class="`plugin-card__security--${assessment.outcome}`"
    :to="detailsTo"
    :title="tooltip"
    :aria-label="`${label}. Open the full review for ${plugin.display_name}`"
  >
    <span aria-hidden="true">{{
      assessment.outcome === 'blocking_findings'
        ? '!'
        : assessment.outcome === 'warnings'
          ? 'i'
          : '✓'
    }}</span>
    {{ label }}
  </NuxtLink>
</template>
