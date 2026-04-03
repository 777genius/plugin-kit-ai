<script setup lang="ts">
import { onMounted, ref } from "vue";
import { register } from "swiper/element/bundle";
import { mdiChevronLeft, mdiChevronRight } from "@mdi/js";

type SwiperContainerElement = HTMLElement & {
  initialize: () => void;
  swiper?: {
    slidePrev: () => void;
    slideNext: () => void;
  };
};

const { content } = useLandingContent();
const { t } = useI18n();

register();

const swiperRef = ref<SwiperContainerElement | null>(null);
const swiperReady = ref(false);
const accents = ["#00f0ff", "#ff00ff", "#39ff14", "#ffd700", "#00f0ff", "#ff00ff"];

onMounted(() => {
  if (!swiperRef.value) return;
  Object.assign(swiperRef.value, {
    slidesPerView: 1.1,
    spaceBetween: 16,
    loop: true,
    grabCursor: true,
    centeredSlides: true,
    pagination: {
      clickable: true
    },
    injectStyles: [`
      .swiper-pagination {
        position: relative !important;
        bottom: auto !important;
        margin-top: 28px;
      }
      .swiper-pagination-bullet {
        width: 10px;
        height: 10px;
        background: rgba(0, 240, 255, 0.4);
        opacity: 1;
      }
      .swiper-pagination-bullet-active {
        background: #00f0ff;
        width: 28px;
        border-radius: 5px;
      }
    `],
    breakpoints: {
      700: {
        slidesPerView: 1.6,
        spaceBetween: 20
      },
      960: {
        slidesPerView: 2.2,
        spaceBetween: 24
      },
      1264: {
        slidesPerView: 2.6,
        spaceBetween: 28
      }
    }
  });
  swiperRef.value.initialize();
  swiperReady.value = true;
});

function slidePrev() {
  swiperRef.value?.swiper?.slidePrev();
}

function slideNext() {
  swiperRef.value?.swiper?.slideNext();
}
</script>

<template>
  <section id="plugins" class="screenshots-section section anchor-offset">
    <v-container>
      <div class="screenshots-section__header">
        <h2 class="screenshots-section__title">
          {{ t("plugins.sectionTitle") }}
        </h2>
        <p class="screenshots-section__subtitle">
          {{ t("plugins.sectionSubtitle") }}
        </p>
      </div>
    </v-container>

    <div class="screenshots-section__carousel-wrap" :class="{ 'is-ready': swiperReady }">
      <swiper-container
        ref="swiperRef"
        init="false"
        class="screenshots-section__swiper"
      >
        <swiper-slide
          v-for="(plugin, idx) in content.plugins"
          :key="plugin.id"
          class="screenshots-section__slide"
        >
          <article class="screenshots-section__card" :style="{ '--accent': accents[idx % accents.length] }">
            <div class="screenshots-section__card-top">
              <div>
                <p class="screenshots-section__eyebrow">{{ plugin.eyebrow }}</p>
                <h3 class="screenshots-section__card-title">{{ plugin.title }}</h3>
              </div>
              <span class="screenshots-section__status">{{ plugin.status }}</span>
            </div>

            <p class="screenshots-section__desc">{{ plugin.description }}</p>

            <div class="screenshots-section__mock">
              <div class="screenshots-section__mock-header">
                <span>{{ t("plugins.preview") }}</span>
                <span>{{ plugin.badges[0] }}</span>
              </div>
              <div class="screenshots-section__mock-lines">
                <div
                  v-for="line in plugin.previewLines"
                  :key="line"
                  class="screenshots-section__mock-line"
                >
                  {{ line }}
                </div>
              </div>
            </div>

            <div class="screenshots-section__badges-label">{{ t("plugins.supports") }}</div>
            <div class="screenshots-section__badges">
              <span
                v-for="badge in plugin.badges"
                :key="badge"
                class="screenshots-section__badge"
              >
                {{ badge }}
              </span>
            </div>
          </article>
        </swiper-slide>
      </swiper-container>

      <button
        class="screenshots-section__nav screenshots-section__nav--prev"
        :aria-label="t('common.previous')"
        @click="slidePrev"
      >
        <v-icon :icon="mdiChevronLeft" size="28" />
      </button>
      <button
        class="screenshots-section__nav screenshots-section__nav--next"
        :aria-label="t('common.next')"
        @click="slideNext"
      >
        <v-icon :icon="mdiChevronRight" size="28" />
      </button>
    </div>
  </section>
</template>

<style scoped>
.screenshots-section {
  position: relative;
}

.screenshots-section__header {
  text-align: center;
  max-width: 640px;
  margin: 0 auto 48px;
  position: relative;
  z-index: 1;
}

.screenshots-section__title {
  font-size: 2.4rem;
  font-weight: 800;
  letter-spacing: -0.03em;
  line-height: 1.15;
  margin-bottom: 16px;
  background: linear-gradient(135deg, #e0e6ff 0%, #00f0ff 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.screenshots-section__subtitle {
  font-size: 1.1rem;
  color: #8892b0;
  line-height: 1.6;
  margin: 0;
}

.screenshots-section__carousel-wrap {
  position: relative;
}

.screenshots-section__swiper {
  padding: 0 16px 8px;
  opacity: 0;
  transition: opacity 0.3s ease;
}

.screenshots-section__carousel-wrap.is-ready .screenshots-section__swiper {
  opacity: 1;
}

.screenshots-section__slide {
  height: auto;
}

.screenshots-section__card {
  position: relative;
  height: 100%;
  min-height: 410px;
  border-radius: 18px;
  overflow: hidden;
  background: rgba(10, 10, 15, 0.8);
  border: 1px solid rgba(0, 240, 255, 0.12);
  transition: transform 0.35s ease, box-shadow 0.35s ease, border-color 0.35s ease;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.3);
  padding: 24px;
  display: flex;
  flex-direction: column;
}

.screenshots-section__card:hover {
  transform: translateY(-6px);
  border-color: color-mix(in srgb, var(--accent) 35%, transparent);
  box-shadow: 0 20px 60px rgba(0, 240, 255, 0.08), 0 12px 32px rgba(0, 0, 0, 0.35);
}

.screenshots-section__nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 3;
  width: 48px;
  height: 48px;
  border-radius: 999px;
  border: 1px solid rgba(0, 240, 255, 0.14);
  background: rgba(10, 10, 15, 0.8);
  color: #e0e6ff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: transform 0.2s ease, background 0.2s ease, border-color 0.2s ease;
}

.screenshots-section__nav:hover {
  transform: translateY(-50%) scale(1.04);
  border-color: rgba(0, 240, 255, 0.28);
}

.screenshots-section__nav--prev {
  left: 16px;
}

.screenshots-section__nav--next {
  right: 16px;
}

.screenshots-section__card-top {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: start;
  margin-bottom: 14px;
}

.screenshots-section__eyebrow {
  margin: 0 0 8px;
  font-size: 0.68rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  font-family: "JetBrains Mono", monospace;
  color: var(--accent);
}

.screenshots-section__card-title {
  margin: 0;
  font-size: 1.2rem;
  line-height: 1.2;
  color: #e0e6ff;
}

.screenshots-section__status {
  flex-shrink: 0;
  border-radius: 999px;
  padding: 7px 10px;
  font-size: 0.68rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  font-weight: 700;
  background: rgba(57, 255, 20, 0.08);
  color: #39ff14;
  border: 1px solid rgba(57, 255, 20, 0.15);
}

.screenshots-section__desc {
  margin: 0 0 18px;
  color: #8892b0;
  line-height: 1.65;
  font-size: 0.94rem;
}

.screenshots-section__mock {
  border-radius: 16px;
  padding: 14px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
  margin-bottom: 18px;
}

.screenshots-section__mock-header {
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

.screenshots-section__mock-lines {
  display: grid;
  gap: 8px;
}

.screenshots-section__mock-line {
  padding: 10px 12px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--accent) 8%, rgba(255, 255, 255, 0.02));
  border: 1px solid color-mix(in srgb, var(--accent) 16%, transparent);
  font-family: "JetBrains Mono", monospace;
  color: #dbeafe;
  font-size: 0.76rem;
}

.screenshots-section__badges-label {
  margin-top: auto;
  margin-bottom: 10px;
  color: #8892b0;
  font-size: 0.72rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  font-family: "JetBrains Mono", monospace;
}

.screenshots-section__badges {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.screenshots-section__badge {
  padding: 8px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: #e0e6ff;
  font-size: 0.76rem;
}

.v-theme--light .screenshots-section__title {
  background: linear-gradient(135deg, #1e293b 0%, #0891b2 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.v-theme--light .screenshots-section__subtitle {
  color: #475569;
}

.v-theme--light .screenshots-section__card {
  background: rgba(255, 255, 255, 0.88);
}

.v-theme--light .screenshots-section__card-title,
.v-theme--light .screenshots-section__badge {
  color: #0f172a;
}

.v-theme--light .screenshots-section__desc,
.v-theme--light .screenshots-section__badges-label,
.v-theme--light .screenshots-section__mock-header {
  color: #64748b;
}

.v-theme--light .screenshots-section__mock-line {
  color: #164e63;
}

@media (max-width: 959px) {
  .screenshots-section__nav {
    display: none;
  }
}

@media (max-width: 600px) {
  .screenshots-section__header {
    margin-bottom: 36px;
  }

  .screenshots-section__title {
    font-size: 1.9rem;
  }

  .screenshots-section__card {
    min-height: 390px;
    padding: 20px;
  }
}
</style>
