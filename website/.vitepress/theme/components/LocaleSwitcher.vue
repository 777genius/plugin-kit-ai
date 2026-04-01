<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute, withBase } from "vitepress";

type Variant = "navbar" | "screen";
type LocaleCode = "en" | "ru";

const props = withDefaults(
  defineProps<{
    variant?: Variant;
  }>(),
  {
    variant: "navbar"
  }
);

const route = useRoute();
const open = ref(false);
const rootEl = ref<HTMLElement | null>(null);

const locales = [
  { code: "ru" as const, title: "Русский", shortTitle: "RU", basePath: "/ru/" },
  { code: "en" as const, title: "English", shortTitle: "EN", basePath: "/en/" }
];

const currentLocale = computed(() => {
  const code = localeFromPath(route.path);
  return locales.find((locale) => locale.code === code) ?? null;
});

const localeLinks = computed(() =>
  locales.map((locale) => ({
    ...locale,
    href: withBase(buildLocalePath(locale.code))
  }))
);

const buttonLabel = computed(() => currentLocale.value?.title || "Language");
const buttonCode = computed(() => currentLocale.value?.shortTitle || "EN");
const currentLocaleCode = computed(() => currentLocale.value?.code || null);

function localeFromPath(path: string): LocaleCode | null {
  const normalized = normalizePath(path);
  if (normalized === "/en" || normalized.startsWith("/en/")) {
    return "en";
  }
  if (normalized === "/ru" || normalized.startsWith("/ru/")) {
    return "ru";
  }
  return null;
}

function normalizePath(path: string): string {
  if (!path || path === "/") {
    return "/";
  }
  return path.replace(/\/+$/, "");
}

function buildLocalePath(target: LocaleCode): string {
  const normalized = normalizePath(route.path);
  if (normalized === "/") {
    return target === "ru" ? "/ru/" : "/en/";
  }

  const current = localeFromPath(normalized);
  if (!current) {
    return target === "ru" ? "/ru/" : "/en/";
  }

  const currentPrefix = `/${current}`;
  const suffix = normalized.slice(currentPrefix.length) || "/";
  const nextPath = `/${target}${suffix === "/" ? "/" : suffix}`;
  return nextPath.endsWith("/") ? nextPath : `${nextPath}/`;
}

function toggle() {
  open.value = !open.value;
}

function close() {
  open.value = false;
}

function handleDocumentClick(event: MouseEvent) {
  if (!(event.target instanceof Node)) {
    return;
  }
  if (!rootEl.value?.contains(event.target)) {
    close();
  }
}

function handleEscape(event: KeyboardEvent) {
  if (event.key === "Escape") {
    close();
  }
}

onMounted(() => {
  document.addEventListener("click", handleDocumentClick);
  document.addEventListener("keydown", handleEscape);
});

onBeforeUnmount(() => {
  document.removeEventListener("click", handleDocumentClick);
  document.removeEventListener("keydown", handleEscape);
});
</script>

<template>
  <div
    ref="rootEl"
    :class="['locale-switcher', `locale-switcher--${props.variant}`, { 'is-open': open }]"
    @mouseenter="props.variant === 'navbar' ? (open = true) : undefined"
    @mouseleave="props.variant === 'navbar' ? (open = false) : undefined"
  >
    <button
      type="button"
      class="locale-switcher__button"
      :aria-expanded="open"
      aria-haspopup="true"
      :aria-label="buttonLabel"
      @click="toggle"
    >
      <span class="vpi-languages locale-switcher__button-icon" />
      <span v-if="props.variant === 'screen'" class="locale-switcher__button-text">{{ buttonLabel }}</span>
      <span v-else class="locale-switcher__button-code">{{ buttonCode }}</span>
      <span class="vpi-chevron-down locale-switcher__button-chevron" />
    </button>

    <div v-if="props.variant === 'navbar'" class="locale-switcher__menu">
      <div class="locale-switcher__panel">
        <p class="locale-switcher__title">{{ buttonLabel }}</p>
        <a
          v-for="locale in localeLinks"
          :key="locale.code"
          class="locale-switcher__link"
          :class="{ 'is-active': locale.code === currentLocaleCode }"
          :href="locale.href"
          :lang="locale.code"
          :hreflang="locale.code"
          @click="close"
        >
          <span>{{ locale.title }}</span>
        </a>
      </div>
    </div>

    <div v-else class="locale-switcher__screen" v-show="open">
      <a
        v-for="locale in localeLinks"
        :key="locale.code"
        class="locale-switcher__screen-link"
        :class="{ 'is-active': locale.code === currentLocaleCode }"
        :href="locale.href"
        :lang="locale.code"
        :hreflang="locale.code"
        @click="close"
      >
        <span>{{ locale.title }}</span>
        <span class="locale-switcher__screen-meta">{{ locale.shortTitle }}</span>
      </a>
    </div>
  </div>
</template>
