<script setup lang="ts">
import { onMounted, ref, watch } from "vue";

const props = defineProps<{
  chart: string;
}>();

const svg = ref("");
const error = ref("");

function nextDiagramId() {
  return `pkai-mermaid-${Math.random().toString(36).slice(2)}`;
}

async function renderDiagram() {
  if (typeof window === "undefined") {
    return;
  }

  try {
    const mermaid = (await import("mermaid")).default;
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: "neutral",
      themeVariables: {
        fontFamily: "ui-sans-serif, system-ui, sans-serif"
      }
    });

    const { svg: rendered } = await mermaid.render(nextDiagramId(), props.chart.trim());
    svg.value = rendered;
    error.value = "";
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : String(cause);
    error.value = `Mermaid render failed: ${message}`;
    svg.value = "";
  }
}

onMounted(() => {
  void renderDiagram();
});

watch(
  () => props.chart,
  () => {
    void renderDiagram();
  }
);
</script>

<template>
  <div class="mermaid-diagram">
    <div v-if="svg" class="mermaid-diagram__surface" v-html="svg" />
    <pre v-else-if="error" class="mermaid-diagram__error">{{ error }}</pre>
  </div>
</template>
