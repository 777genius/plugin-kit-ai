<script setup lang="ts">
import { mdiClose, mdiGithub, mdiMenu } from "@mdi/js";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const localePath = useLocalePath();
const config = useRuntimeConfig();
const menuOpen = ref(false);
const interactiveReady = ref(false);
const githubUrl = `https://github.com/${config.public.githubRepo}`;
const homePath = computed(() => localePath("/"));
const homeHref = computed(() => router.resolve(homePath.value).href);

const navItems = computed(() => [
  { id: "features", label: t("nav.features") },
  { id: "plugins", label: t("nav.plugins") },
  { id: "download", label: t("nav.download") },
  { id: "comparison", label: t("nav.comparison") },
  { id: "faq", label: t("nav.faq") }
]);

const normalizePath = (value: string) => (value !== "/" ? value.replace(/\/+$/, "") : "/");

const isHomePage = computed(
  () => normalizePath(route.path) === normalizePath(homePath.value)
);

const sectionHref = (sectionId: string) =>
  isHomePage.value ? `#${sectionId}` : `${homeHref.value}#${sectionId}`;

onMounted(() => {
  interactiveReady.value = true;
});
</script>

<template>
  <header class="app-header">
    <v-container class="app-header__inner">
      <AppLogo />
      <nav class="app-header__nav">
        <v-btn v-for="item in navItems" :key="item.id" variant="text" :href="sectionHref(item.id)">
          {{ item.label }}
        </v-btn>
      </nav>
      <div class="app-header__spacer" />
      <div class="app-header__desktop-actions">
        <template v-if="interactiveReady">
          <LanguageSwitcher icon-only />
        </template>
        <div v-else class="app-header__control-fallback" aria-hidden="true" />
        <v-btn
          variant="outlined"
          size="small"
          :href="githubUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="app-header__github-btn"
          :prepend-icon="mdiGithub"
        >
          {{ t("nav.viewOnGithub") }}
        </v-btn>
        <template v-if="interactiveReady">
          <ThemeToggle />
        </template>
        <div v-else class="app-header__control-fallback" aria-hidden="true" />
      </div>
      <div class="app-header__mobile-actions">
        <v-btn :icon="mdiMenu" variant="text" @click="menuOpen = true" />
        <Teleport to="body">
          <Transition name="mobile-menu-fade">
            <div v-if="menuOpen" class="mobile-menu-overlay" @click.self="menuOpen = false">
              <div class="mobile-menu">
                <div class="mobile-menu__header">
                  <div @click="menuOpen = false">
                    <AppLogo />
                  </div>
                  <div style="flex: 1" />
                  <v-btn :icon="mdiClose" variant="text" @click="menuOpen = false" />
                </div>
                <hr class="mobile-menu__divider">
                <nav class="mobile-menu__list">
                  <a
                    v-for="item in navItems"
                    :key="item.id"
                    :href="sectionHref(item.id)"
                    class="mobile-menu__link"
                    @click="menuOpen = false"
                  >
                    {{ item.label }}
                  </a>
                  <a
                    :href="githubUrl"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="mobile-menu__link"
                    @click="menuOpen = false"
                  >
                    {{ t("nav.viewOnGithub") }}
                  </a>
                </nav>
                <hr class="mobile-menu__divider">
                <div class="mobile-menu__actions">
                  <template v-if="interactiveReady">
                    <LanguageSwitcher compact />
                    <ThemeToggle />
                  </template>
                  <template v-else>
                    <div class="app-header__control-fallback app-header__control-fallback--wide" aria-hidden="true" />
                    <div class="app-header__control-fallback" aria-hidden="true" />
                  </template>
                </div>
              </div>
            </div>
          </Transition>
        </Teleport>
      </div>
    </v-container>
  </header>
</template>

<style scoped>
.app-header {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 1000;
  height: 64px;
  display: flex;
  align-items: center;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-bottom: 1px solid rgba(0, 240, 255, 0.08);
}

.v-theme--light .app-header {
  background: rgba(255, 255, 255, 0.9);
  border-bottom-color: rgba(0, 0, 0, 0.06);
}

.v-theme--dark .app-header {
  background: rgba(10, 10, 15, 0.9);
}

.app-header__inner {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
}

.app-header__nav {
  display: flex;
  align-self: stretch;
  align-items: stretch;
  margin-left: 48px;
}

.app-header__nav :deep(.v-btn) {
  height: 100% !important;
  border-radius: 0;
}

.app-header__spacer {
  flex: 1;
}

.app-header__desktop-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.app-header__control-fallback {
  width: 40px;
  height: 36px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.app-header__control-fallback--wide {
  width: 96px;
  border-radius: 12px;
}

.v-theme--light .app-header__control-fallback {
  background: rgba(15, 23, 42, 0.04);
  border-color: rgba(15, 23, 42, 0.08);
}


.app-header__github-btn {
  border-color: rgba(0, 240, 255, 0.25) !important;
  color: #00f0ff !important;
  font-weight: 600 !important;
  font-size: 12px !important;
  letter-spacing: 0.02em !important;
}

.app-header__github-btn:hover {
  border-color: rgba(0, 240, 255, 0.5) !important;
  background: rgba(0, 240, 255, 0.06) !important;
}

.app-header__mobile-actions {
  display: none;
}

@media (max-width: 959px) {
  .app-header__nav {
    display: none;
  }

  .app-header__desktop-actions {
    display: none;
  }

  .app-header__mobile-actions {
    display: flex;
  }
}

.mobile-menu-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: rgb(var(--v-theme-surface));
}

.mobile-menu {
  padding: 16px 16px 24px;
  height: 100%;
  overflow-y: auto;
}

.mobile-menu__header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 12px;
}

.mobile-menu__divider {
  border: none;
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.mobile-menu__list {
  display: flex;
  flex-direction: column;
  padding: 8px 0;
}

.mobile-menu__link {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  font-size: 1rem;
  color: rgb(var(--v-theme-on-surface));
  text-decoration: none;
  border-radius: 8px;
  transition: background-color 0.15s;
}

.mobile-menu__link:hover {
  background: rgba(var(--v-theme-on-surface), 0.06);
}

.mobile-menu__actions {
  display: flex;
  flex-direction: row;
  gap: 8px;
  align-items: center;
  justify-content: center;
  padding-top: 16px;
}

.mobile-menu-fade-enter-active,
.mobile-menu-fade-leave-active {
  transition: opacity 0.2s ease;
}

.mobile-menu-fade-enter-from,
.mobile-menu-fade-leave-to {
  opacity: 0;
}
</style>
