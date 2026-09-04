<script setup lang="ts">
import { clients } from '~/data/clients';

const { asset } = useSite();
const field = ref<HTMLElement>();
const inView = ref(false);
const pageVisible = ref(true);
let observer: IntersectionObserver | undefined;
const updatePageVisibility = () => {
  pageVisible.value = !document.hidden;
};

onMounted(() => {
  updatePageVisibility();
  document.addEventListener('visibilitychange', updatePageVisibility);
  if (field.value && 'IntersectionObserver' in window) {
    observer = new IntersectionObserver(([entry]) => {
      inView.value = Boolean(entry?.isIntersecting);
    });
    observer.observe(field.value);
  } else {
    inView.value = true;
  }
});

onBeforeUnmount(() => {
  observer?.disconnect();
  document.removeEventListener('visibilitychange', updatePageVisibility);
});
</script>

<template>
  <div
    ref="field"
    class="hero-agent-field"
    :class="{ 'hero-agent-field--active': inView && pageVisible }"
    aria-hidden="true"
  >
    <div class="hero-agent-field__plane">
      <div class="hero-agent-field__track">
        <span
          v-for="(client, index) in clients"
          :key="client.id"
          class="hero-agent-field__spoke"
          :data-client-id="client.id"
          :style="{ '--angle': `${(index * 360) / clients.length}deg` }"
        >
          <span class="hero-agent-field__node">
            <span class="hero-agent-field__face">
              <img :src="asset(`client-icons/${client.icon}`)" alt="" width="26" height="26" >
            </span>
          </span>
        </span>
      </div>
      <span class="hero-agent-field__hub">
        <img :src="asset('icon.svg')" alt="" width="38" height="38" >
      </span>
    </div>
  </div>
</template>

<style src="~/assets/styles/hero-agent-field.scss" lang="scss" />
