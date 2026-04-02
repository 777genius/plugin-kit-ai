<script setup lang="ts">
const localePath = useLocalePath();
const homePath = computed(() => localePath("/"));
</script>

<template>
  <NuxtLink :to="homePath" class="app-logo">
    <span class="app-logo__mark">H</span>
    <span class="app-logo__copy">
      <span class="app-logo__text">Hookplex</span>
      <span class="app-logo__subtext">plugin-kit-ai</span>
    </span>
  </NuxtLink>
</template>

<style scoped>
.app-logo {
  font-weight: 700;
  text-decoration: none;
  color: inherit;
  display: inline-flex;
  align-items: center;
  gap: 12px;
  line-height: 1;
  align-self: center;
}

.app-logo__mark {
  width: 36px;
  height: 36px;
  border-radius: 11px;
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-family: "JetBrains Mono", monospace;
  font-weight: 800;
  font-size: 16px;
  color: #0a0a0f;
  background: linear-gradient(135deg, #00f0ff, #ff00ff 55%, #39ff14);
  box-shadow: 0 8px 24px rgba(0, 240, 255, 0.2);
}

.app-logo__copy {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.app-logo__text {
  font-family: "JetBrains Mono", monospace;
  font-weight: 700;
  font-size: 14px;
  letter-spacing: 0.04em;
  background: linear-gradient(135deg, #ffffff, #00f0ff);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.app-logo__subtext {
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #8892b0;
}
</style>

<style>
.v-theme--light .app-logo__mark {
  color: #ffffff;
  box-shadow: 0 8px 20px rgba(8, 145, 178, 0.2);
}

.v-theme--light .app-logo__text {
  background: linear-gradient(135deg, #1e293b, #0891b2);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.v-theme--light .app-logo__subtext {
  color: #64748b;
}
</style>
