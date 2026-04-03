<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

const { t } = useI18n();

const steps = computed(() => [
  { id: "author", label: t("hero.demo.steps.author"), accent: "#00f0ff" },
  { id: "render", label: t("hero.demo.steps.render"), accent: "#ff00ff" },
  { id: "validate", label: t("hero.demo.steps.validate"), accent: "#ffd700" },
  { id: "ship", label: t("hero.demo.steps.ship"), accent: "#39ff14" }
]);

const outputs = ["Claude", "Codex", "Gemini", "OpenCode", "Cursor"];
const logs = computed(() => [
  t("hero.demo.logs.author"),
  t("hero.demo.logs.render"),
  t("hero.demo.logs.validate"),
  t("hero.demo.logs.ship")
]);

const containerRef = ref<HTMLElement | null>(null);
const activeStep = ref(0);
const activeLog = ref(logs.value[0]);

let observer: IntersectionObserver | null = null;
let intervalId: ReturnType<typeof setInterval> | null = null;

const start = () => {
  if (intervalId) return;
  intervalId = setInterval(() => {
    activeStep.value = (activeStep.value + 1) % steps.value.length;
    activeLog.value = logs.value[activeStep.value];
  }, 1800);
};

const stop = () => {
  if (!intervalId) return;
  clearInterval(intervalId);
  intervalId = null;
};

const visible = ref(false);

watch(visible, (value) => {
  if (value) start();
  else stop();
});

watch(logs, (nextLogs) => {
  activeLog.value = nextLogs[activeStep.value] || nextLogs[0] || "";
});

onMounted(() => {
  observer = new IntersectionObserver(
    ([entry]) => {
      visible.value = entry.isIntersecting;
    },
    { threshold: 0.15 }
  );
  if (containerRef.value) observer.observe(containerRef.value);
});

onUnmounted(() => {
  observer?.disconnect();
  stop();
});
</script>

<template>
  <div ref="containerRef" class="hero-demo" role="img" :aria-label="t('hero.preview')">
    <div class="hero-demo__content">
      <div class="hero-demo__header">
        <div class="hero-demo__title-row">
          <span class="hero-demo__title">{{ t("hero.demo.title") }}</span>
          <span class="hero-demo__badge-live">
            <span class="hero-demo__live-dot" />
            {{ t("hero.demo.live") }}
          </span>
        </div>
        <p class="hero-demo__subtitle">{{ t("hero.demo.subtitle") }}</p>
      </div>

      <div class="hero-demo__steps">
        <div
          v-for="(step, index) in steps"
          :key="step.id"
          class="hero-demo__step"
          :class="{ 'hero-demo__step--active': index === activeStep }"
          :style="{ '--accent': step.accent }"
        >
          <div class="hero-demo__step-dot" />
          <div class="hero-demo__step-copy">
            <span class="hero-demo__step-label">{{ step.label }}</span>
            <span class="hero-demo__step-state">{{ index <= activeStep ? t("hero.demo.ready") : t("hero.demo.waiting") }}</span>
          </div>
        </div>
      </div>

      <div class="hero-demo__files">
        <div class="hero-demo__file-card">
          <div class="hero-demo__file-header">
            <span>{{ t("hero.demo.repo") }}</span>
            <span>{{ t("hero.demo.sourceOfTruth") }}</span>
          </div>
          <div class="hero-demo__file-list">
            <div class="hero-demo__file">plugin.yaml</div>
            <div class="hero-demo__file">targets/claude/</div>
            <div class="hero-demo__file">targets/codex/</div>
            <div class="hero-demo__file">README.md</div>
          </div>
        </div>

        <div class="hero-demo__output-card">
          <div class="hero-demo__file-header">
            <span>{{ t("hero.demo.outputs") }}</span>
            <span>{{ t("hero.demo.supportedAgents") }}</span>
          </div>
          <div class="hero-demo__output-list">
            <span
              v-for="output in outputs"
              :key="output"
              class="hero-demo__output hero-demo__output--active"
            >
              {{ output }}
            </span>
          </div>
        </div>
      </div>

      <div class="hero-demo__log">
        {{ activeLog }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.hero-demo {
  position: relative;
  border-radius: 18px;
  background: rgba(10, 10, 15, 0.82);
  border: 1px solid rgba(0, 240, 255, 0.14);
  box-shadow:
    0 24px 80px rgba(0, 0, 0, 0.35),
    0 0 60px rgba(0, 240, 255, 0.05);
  overflow: hidden;
}

.hero-demo__content {
  padding: 22px;
}

.hero-demo__header {
  margin-bottom: 18px;
}

.hero-demo__title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.hero-demo__title {
  font-family: "JetBrains Mono", monospace;
  font-size: 0.95rem;
  color: #e0e6ff;
}

.hero-demo__subtitle {
  margin: 10px 0 0;
  color: #8892b0;
  font-size: 0.88rem;
}

.hero-demo__badge-live {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 0.68rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #39ff14;
  border: 1px solid rgba(57, 255, 20, 0.18);
  background: rgba(57, 255, 20, 0.05);
}

.hero-demo__live-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #39ff14;
  box-shadow: 0 0 12px #39ff14;
}

.hero-demo__steps {
  display: grid;
  gap: 10px;
  margin-bottom: 18px;
}

.hero-demo__step {
  display: flex;
  align-items: center;
  gap: 12px;
  border-radius: 14px;
  padding: 12px 14px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.hero-demo__step--active {
  border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  background: color-mix(in srgb, var(--accent) 8%, rgba(255, 255, 255, 0.02));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 20%, transparent);
}

.hero-demo__step-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 16px var(--accent);
}

.hero-demo__step-copy {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.hero-demo__step-label {
  color: #e0e6ff;
  font-family: "JetBrains Mono", monospace;
  font-size: 0.83rem;
}

.hero-demo__step-state {
  color: #8892b0;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  font-size: 0.64rem;
}

.hero-demo__files {
  display: grid;
  grid-template-columns: 1.1fr 1fr;
  gap: 14px;
  margin-bottom: 18px;
}

.hero-demo__file-card,
.hero-demo__output-card {
  border-radius: 16px;
  padding: 14px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.hero-demo__file-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  color: #8892b0;
  font-size: 0.68rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  font-family: "JetBrains Mono", monospace;
}

.hero-demo__file-list {
  display: grid;
  gap: 8px;
}

.hero-demo__file {
  padding: 9px 10px;
  border-radius: 12px;
  background: rgba(0, 240, 255, 0.06);
  border: 1px solid rgba(0, 240, 255, 0.08);
  color: #dbeafe;
  font-family: "JetBrains Mono", monospace;
  font-size: 0.78rem;
}

.hero-demo__output-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.hero-demo__output {
  padding: 8px 10px;
  border-radius: 999px;
  font-size: 0.76rem;
  color: #8892b0;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.hero-demo__output--active {
  color: #0a0a0f;
  background: linear-gradient(135deg, #00f0ff, #39ff14);
  border-color: transparent;
}

.hero-demo__log {
  border-radius: 14px;
  padding: 12px 14px;
  background: rgba(5, 8, 18, 0.65);
  border: 1px solid rgba(57, 255, 20, 0.12);
  color: #b8c3e0;
  font-size: 0.82rem;
  line-height: 1.5;
}

.v-theme--light .hero-demo {
  background: rgba(255, 255, 255, 0.92);
  border-color: rgba(8, 145, 178, 0.14);
}

.v-theme--light .hero-demo__title,
.v-theme--light .hero-demo__step-label {
  color: #0f172a;
}

.v-theme--light .hero-demo__file {
  color: #164e63;
}

.v-theme--light .hero-demo__log {
  background: rgba(241, 245, 249, 0.9);
  color: #334155;
  border-color: rgba(34, 197, 94, 0.18);
}

@media (max-width: 700px) {
  .hero-demo__files {
    grid-template-columns: 1fr;
  }

  .hero-demo__step-copy {
    flex-direction: column;
    gap: 4px;
  }
}
</style>
