<script setup lang="ts">
import { onMounted, watch } from "vue";
import { useData, useRoute } from "vitepress";

const storageKey = "plugin-kit-ai-docs-locale";
const { site } = useData();
const route = useRoute();

function localeFromPath(path: string): string | null {
  const normalized = stripBase(path === "/" ? "/" : path.replace(/\/+$/, ""));
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

function stripBase(path: string): string {
  const base = normalizePath(site.value.base || "/");
  if (base === "/" || !path.startsWith(base)) {
    return path;
  }
  const stripped = path.slice(base.length);
  return stripped.startsWith("/") ? stripped : `/${stripped}`;
}

function persistLocale(path: string) {
  if (typeof window === "undefined") {
    return;
  }
  const locale = localeFromPath(path);
  if (!locale) {
    return;
  }
  try {
    window.localStorage.setItem(storageKey, locale);
  } catch {
    // localStorage is optional enhancement only.
  }
}

onMounted(() => {
  persistLocale(route.path);
  watch(
    () => route.path,
    (path) => persistLocale(path)
  );
});
</script>

<template>
  <span hidden aria-hidden="true"></span>
</template>
