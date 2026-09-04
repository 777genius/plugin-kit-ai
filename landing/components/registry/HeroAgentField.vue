<script setup lang="ts">
import { clients } from '~/data/clients';
import type { HeroScene } from '~/utils/heroScene';

const { asset } = useSite();
const field = ref<HTMLElement>();
const state = ref('fallback');
const running = ref(false);
let scene: HeroScene | undefined;
let observer: IntersectionObserver | undefined;
let resizeObserver: ResizeObserver | undefined;
let media: MediaQueryList;
let motion: MediaQueryList;
let connection: (EventTarget & { saveData?: boolean }) | undefined;
let visible = false;
let disposed = false;
let failed = false;
let generation = 0;
let setup: AbortController | undefined;
let idle: number | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;
let pointerTarget: HTMLElement | null = null;

const eligible = () => media.matches && !motion.matches && !connection?.saveData;
const active = () => !disposed && eligible() && visible && !document.hidden;
const cancelScheduled = () => {
  if (idle !== undefined) window.cancelIdleCallback(idle);
  if (timer !== undefined) clearTimeout(timer);
  idle = undefined;
  timer = undefined;
};
const fail = () => {
  failed = true;
  generation++;
  scene?.dispose();
  scene = undefined;
  state.value = 'fallback';
  running.value = false;
};

async function initialize() {
  cancelScheduled();
  if (!active() || scene || failed || state.value === 'loading') return;
  state.value = 'loading';
  const token = ++generation;
  setup = new AbortController();
  try {
    const { createHeroScene } = await import('~/utils/heroScene');
    if (token !== generation || !active() || !field.value) {
      if (!disposed && token === generation) state.value = 'fallback';
      return;
    }
    const next = await createHeroScene(
      field.value,
      clients.map((client) => asset(`client-icons/${client.icon}`)),
      asset('icon.svg'),
      fail,
      setup.signal,
    );
    if (token !== generation || !active()) {
      next.dispose();
      if (!disposed && token === generation) state.value = 'fallback';
      return;
    }
    scene = next;
    scene.resize();
    scene.setRunning(true);
    state.value = 'ready';
    running.value = true;
  } catch {
    if (!disposed && token === generation) fail();
  }
}

function reconcile() {
  if (disposed) return;
  const shouldRun = active();
  scene?.setRunning(shouldRun);
  running.value = Boolean(scene && shouldRun);
  if (!eligible()) {
    generation++;
    setup?.abort();
    scene?.dispose();
    scene = undefined;
    state.value = 'fallback';
  }
  if (!shouldRun) {
    cancelScheduled();
    if (state.value === 'loading') {
      generation++;
      setup?.abort();
      state.value = 'fallback';
    }
    return;
  }
  if (scene || failed || state.value === 'loading' || idle !== undefined || timer !== undefined)
    return;
  if (document.readyState !== 'complete') return;
  if ('requestIdleCallback' in window)
    idle = window.requestIdleCallback(() => void initialize(), { timeout: 2000 });
  else timer = setTimeout(() => void initialize(), 150);
}

function pointer(event: PointerEvent) {
  if (!running.value || !pointerTarget) return;
  const rect = pointerTarget.getBoundingClientRect();
  scene?.setPointer(
    (event.clientX - rect.left) / rect.width - 0.5,
    (event.clientY - rect.top) / rect.height - 0.5,
  );
}
const resetPointer = () => scene?.setPointer(0, 0);

onMounted(() => {
  media = window.matchMedia('(min-width: 721px)');
  motion = window.matchMedia('(prefers-reduced-motion: reduce)');
  connection = (navigator as Navigator & { connection?: typeof connection }).connection;
  media.addEventListener('change', reconcile);
  motion.addEventListener('change', reconcile);
  connection?.addEventListener('change', reconcile);
  document.addEventListener('visibilitychange', reconcile);
  window.addEventListener('load', reconcile);
  if (!field.value) return;
  pointerTarget = field.value.closest('.hero__demo');
  pointerTarget?.addEventListener('pointermove', pointer);
  pointerTarget?.addEventListener('pointerleave', resetPointer);
  resizeObserver = new ResizeObserver(() => scene?.resize());
  resizeObserver.observe(field.value);
  observer = new IntersectionObserver(([entry]) => {
    visible = Boolean(entry?.isIntersecting);
    reconcile();
  });
  observer.observe(field.value);
});

onBeforeUnmount(() => {
  disposed = true;
  generation++;
  setup?.abort();
  cancelScheduled();
  observer?.disconnect();
  resizeObserver?.disconnect();
  media?.removeEventListener('change', reconcile);
  motion?.removeEventListener('change', reconcile);
  connection?.removeEventListener('change', reconcile);
  document.removeEventListener('visibilitychange', reconcile);
  window.removeEventListener('load', reconcile);
  pointerTarget?.removeEventListener('pointermove', pointer);
  pointerTarget?.removeEventListener('pointerleave', resetPointer);
  scene?.dispose();
});
</script>

<template>
  <div
    ref="field"
    class="hero-agent-field"
    :data-state="state"
    :data-running="String(running)"
    aria-hidden="true"
  >
    <div class="hero-agent-field__fallback">
      <span class="hero-agent-field__ring hero-agent-field__ring--one" />
      <span class="hero-agent-field__ring hero-agent-field__ring--two" />
      <span class="hero-agent-field__ring hero-agent-field__ring--three" />
      <span class="hero-agent-field__hub"
        ><img :src="asset('icon.svg')" alt="" width="64" height="64"
      ></span>
      <span
        v-for="(client, index) in clients"
        :key="client.id"
        class="hero-agent-field__node"
        :data-client-id="client.id"
        :style="{
          left: `${50 + 43 * Math.cos((index * 2 * Math.PI) / clients.length)}%`,
          top: `${50 + 34 * Math.sin((index * 2 * Math.PI) / clients.length)}%`,
        }"
        ><img :src="asset(`client-icons/${client.icon}`)" alt="" width="26" height="26"
      ></span>
    </div>
  </div>
</template>

<style src="~/assets/styles/hero-agent-field.scss" lang="scss" />
