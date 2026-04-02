<script setup lang="ts">
const { content } = useLandingContent()
const { t, locale } = useI18n()
const { data: releaseData, fallbackUrl } = useReleaseDownloads()
const { quickstartUrl, supportBoundaryUrl } = useDocsLinks()

const releaseVersion = computed(() => releaseData.value?.version || null)
const releaseDate = computed(() => {
  if (!releaseData.value?.pubDate) {
    return ""
  }

  return new Date(releaseData.value.pubDate).toLocaleDateString(locale.value, {
    year: "numeric",
    month: "short",
    day: "numeric"
  })
})

const supportAccent = ["#39ff14", "#00f0ff", "#ffb703", "#f472b6", "#94a3b8"]

const installChannels = computed(() =>
  content.value.installChannels.map((channel) =>
    channel.id === "docs"
      ? { ...channel, href: quickstartUrl.value }
      : channel
  )
)
</script>

<template>
  <section id="download" class="download-section section anchor-offset">
    <v-container>
      <div class="download-section__header">
        <h2 class="download-section__title">{{ content.download.title }}</h2>
        <p class="download-section__subtitle">{{ content.download.note }}</p>
        <p v-if="releaseVersion" class="download-section__release-info">
          {{ t("download.latestRelease") }} ·
          <a :href="fallbackUrl" target="_blank" rel="noopener noreferrer">v{{ releaseVersion }}</a>
          <template v-if="releaseDate"> · {{ releaseDate }}</template>
        </p>
      </div>

      <div class="download-section__overview">
        <article class="download-section__overview-card download-section__overview-card--quickstart">
          <div class="download-section__overview-top">
            <h3 class="download-section__overview-title">
              {{ t("download.quickstartTitle") }}
            </h3>
            <p class="download-section__overview-subtitle">
              {{ t("download.quickstartSubtitle") }}
            </p>
          </div>

          <div class="download-section__steps">
            <div
              v-for="(step, index) in content.quickstartSteps"
              :key="step.id"
              class="download-section__step"
            >
              <div class="download-section__step-index">0{{ index + 1 }}</div>
              <div class="download-section__step-body">
                <h4 class="download-section__step-title">{{ step.title }}</h4>
                <code class="download-section__step-command">{{ step.command }}</code>
                <p class="download-section__step-note">{{ step.note }}</p>
              </div>
            </div>
          </div>
        </article>

        <article class="download-section__overview-card">
          <div class="download-section__overview-top">
            <h3 class="download-section__overview-title">
              {{ t("download.supportTitle") }}
            </h3>
            <p class="download-section__overview-subtitle">
              {{ t("download.supportSubtitle") }}
            </p>
          </div>

          <div class="download-section__support-list">
            <div
              v-for="(lane, index) in content.supportLanes"
              :key="lane.id"
              class="download-section__support-item"
              :style="{ '--accent': supportAccent[index % supportAccent.length] }"
            >
              <div class="download-section__support-main">
                <h4 class="download-section__support-name">{{ lane.name }}</h4>
                <span class="download-section__support-status">{{ lane.status }}</span>
              </div>
              <p class="download-section__support-note">{{ lane.note }}</p>
            </div>
          </div>

          <a
            class="download-section__support-link"
            :href="supportBoundaryUrl"
            target="_blank"
            rel="noopener noreferrer"
          >
            {{ t("download.supportLink") }}
          </a>
        </article>
      </div>

      <div class="download-section__cards">
        <div
          v-for="(channel, index) in installChannels"
          :key="channel.id"
          class="download-section__card"
          :class="{ 'download-section__card--active': channel.recommended }"
          :style="{
            '--delay': `${index * 0.1}s`,
            '--accent': index % 3 === 0 ? '#00f0ff' : index % 3 === 1 ? '#ff00ff' : '#39ff14'
          }"
        >
          <div class="download-section__card-glow" />
          <div class="download-section__card-top">
            <h3 class="download-section__card-label">{{ channel.title }}</h3>
            <span v-if="channel.recommended" class="download-section__recommended">
              {{ t("download.recommended") }}
            </span>
          </div>

          <p class="download-section__card-description">{{ channel.description }}</p>

          <div v-if="channel.command" class="download-section__command-wrap">
            <span class="download-section__command-label">{{ t("download.command") }}</span>
            <code class="download-section__command">{{ channel.command }}</code>
          </div>

          <p class="download-section__card-note">{{ channel.note }}</p>

          <a class="download-section__btn" :href="channel.href" target="_blank" rel="noopener noreferrer">
            <span>{{ t("download.open") }}</span>
          </a>
        </div>
      </div>
    </v-container>
  </section>
</template>

<style scoped>
.download-section {
  position: relative;
}

.download-section__header {
  text-align: center;
  max-width: 620px;
  margin: 0 auto 56px;
  position: relative;
  z-index: 1;
}

.download-section__title {
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

.download-section__subtitle {
  font-size: 1.1rem;
  color: #8892b0;
  line-height: 1.6;
  margin: 0;
}

.download-section__release-info {
  text-align: center;
  margin: 16px 0 0;
  font-size: 0.82rem;
  color: #8892b0;
  font-family: "JetBrains Mono", monospace;
}

.download-section__release-info a {
  color: #00f0ff;
  text-decoration: none;
}

.download-section__release-info a:hover {
  text-decoration: underline;
}

.download-section__overview {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 0.85fr);
  gap: 20px;
  max-width: 1040px;
  margin: 0 auto 24px;
  position: relative;
  z-index: 1;
}

.download-section__overview-card {
  border-radius: 20px;
  border: 1px solid rgba(0, 240, 255, 0.1);
  background: rgba(10, 10, 15, 0.82);
  backdrop-filter: blur(16px);
  padding: 24px;
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.2);
}

.download-section__overview-card--quickstart {
  background: linear-gradient(180deg, rgba(0, 240, 255, 0.07), rgba(10, 10, 15, 0.82));
}

.download-section__overview-top {
  margin-bottom: 18px;
}

.download-section__overview-title {
  margin: 0 0 8px;
  font-size: 1.2rem;
  font-weight: 700;
  color: #e0e6ff;
}

.download-section__overview-subtitle {
  margin: 0;
  font-size: 0.92rem;
  line-height: 1.6;
  color: #95a3c4;
}

.download-section__steps {
  display: grid;
  gap: 14px;
}

.download-section__step {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}

.download-section__step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 40px;
  border-radius: 12px;
  border: 1px solid rgba(0, 240, 255, 0.16);
  background: rgba(0, 240, 255, 0.06);
  color: #00f0ff;
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  font-family: "JetBrains Mono", monospace;
}

.download-section__step-body {
  display: grid;
  gap: 8px;
}

.download-section__step-title {
  margin: 0;
  font-size: 0.98rem;
  color: #e0e6ff;
}

.download-section__step-command {
  display: block;
  padding: 12px;
  border-radius: 12px;
  background: rgba(0, 0, 0, 0.24);
  border: 1px solid rgba(255, 255, 255, 0.06);
  color: #dbeafe;
  font-size: 0.8rem;
  line-height: 1.6;
  white-space: pre-wrap;
  font-family: "JetBrains Mono", monospace;
}

.download-section__step-note {
  margin: 0;
  color: #95a3c4;
  line-height: 1.55;
  font-size: 0.85rem;
}

.download-section__support-list {
  display: grid;
  gap: 12px;
}

.download-section__support-link {
  display: inline-flex;
  align-items: center;
  margin-top: 16px;
  color: #00f0ff;
  text-decoration: none;
  font-size: 0.85rem;
  font-weight: 600;
}

.download-section__support-link:hover {
  text-decoration: underline;
}

.download-section__support-item {
  padding: 14px 16px;
  border-radius: 16px;
  border: 1px solid color-mix(in srgb, var(--accent) 18%, transparent);
  background: color-mix(in srgb, var(--accent) 7%, rgba(255, 255, 255, 0.02));
}

.download-section__support-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}

.download-section__support-name {
  margin: 0;
  font-size: 0.96rem;
  font-weight: 700;
  color: #e0e6ff;
}

.download-section__support-status {
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 0.68rem;
  line-height: 1;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  font-weight: 700;
  color: var(--accent);
  border: 1px solid color-mix(in srgb, var(--accent) 32%, transparent);
  background: color-mix(in srgb, var(--accent) 8%, transparent);
}

.download-section__support-note {
  margin: 0;
  color: #95a3c4;
  font-size: 0.85rem;
  line-height: 1.55;
}

.download-section__cards {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
  position: relative;
  z-index: 1;
  max-width: 1040px;
  margin: 0 auto;
  overflow: visible;
  padding: 12px 0;
  align-items: stretch;
}

.download-section__card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  text-align: left;
  padding: 24px 22px 22px;
  border-radius: 16px;
  background: rgba(10, 10, 15, 0.8);
  border: 1px solid rgba(0, 240, 255, 0.08);
  backdrop-filter: blur(16px);
  cursor: default;
  transition:
    transform 0.35s cubic-bezier(0.4, 0, 0.2, 1),
    box-shadow 0.35s cubic-bezier(0.4, 0, 0.2, 1),
    border-color 0.35s ease;
  overflow: hidden;
  animation: downloadFadeUp 0.5s ease both;
  animation-delay: var(--delay, 0s);
}

.download-section__card:hover {
  transform: translateY(-6px);
  border-color: rgba(0, 240, 255, 0.2);
  box-shadow:
    0 20px 60px rgba(0, 240, 255, 0.08),
    0 4px 16px rgba(0, 0, 0, 0.2);
}

.download-section__card--active {
  border-color: rgba(57, 255, 20, 0.28);
  background: rgba(57, 255, 20, 0.05);
  box-shadow:
    0 8px 32px rgba(57, 255, 20, 0.1),
    0 0 0 2px rgba(57, 255, 20, 0.15);
  transform: scale(1.02);
  z-index: 2;
}

.download-section__card--active:hover {
  transform: scale(1.04);
  border-color: rgba(57, 255, 20, 0.5);
  box-shadow:
    0 20px 60px rgba(57, 255, 20, 0.15),
    0 0 0 2px rgba(57, 255, 20, 0.2);
}

.download-section__card-glow {
  position: absolute;
  inset: 0;
  background: radial-gradient(
    ellipse 80% 60% at 50% 0%,
    color-mix(in srgb, var(--accent) 8%, transparent),
    transparent 70%
  );
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.35s ease;
}

.download-section__card:hover .download-section__card-glow {
  opacity: 1;
}

.download-section__card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.download-section__card-label {
  font-size: 1.08rem;
  font-weight: 700;
  margin: 0;
  color: #e0e6ff;
}

.download-section__recommended {
  border-radius: 999px;
  padding: 7px 10px;
  font-size: 0.68rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  font-weight: 700;
  color: #39ff14;
  background: rgba(57, 255, 20, 0.06);
  border: 1px solid rgba(57, 255, 20, 0.18);
}

.download-section__card-description {
  margin: 0 0 14px;
  color: #8892b0;
  line-height: 1.6;
  font-size: 0.92rem;
}

.download-section__command-wrap {
  margin-bottom: 14px;
}

.download-section__command-label {
  display: block;
  margin-bottom: 8px;
  color: #8892b0;
  font-size: 0.7rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  font-family: "JetBrains Mono", monospace;
}

.download-section__command {
  display: block;
  padding: 12px;
  border-radius: 12px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.06);
  color: #dbeafe;
  font-size: 0.8rem;
  line-height: 1.55;
  word-break: break-word;
  font-family: "JetBrains Mono", monospace;
}

.download-section__card-note {
  margin: 0 0 16px;
  color: #a8b3d1;
  font-size: 0.85rem;
  line-height: 1.55;
}

.download-section__btn {
  margin-top: auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  text-decoration: none;
  width: 100%;
  padding: 12px 14px;
  border-radius: 12px;
  font-weight: 700;
  color: #0a0a0f;
  background: linear-gradient(135deg, var(--accent), color-mix(in srgb, var(--accent) 70%, #ffffff));
  box-shadow: 0 8px 24px color-mix(in srgb, var(--accent) 25%, transparent);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.download-section__btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 12px 32px color-mix(in srgb, var(--accent) 35%, transparent);
}

@keyframes downloadFadeUp {
  from {
    opacity: 0;
    transform: translateY(16px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.v-theme--light .download-section__title {
  background: linear-gradient(135deg, #1e293b 0%, #0891b2 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.v-theme--light .download-section__subtitle,
.v-theme--light .download-section__release-info {
  color: #475569;
}

.v-theme--light .download-section__overview-card {
  background: rgba(255, 255, 255, 0.9);
  border-color: rgba(15, 23, 42, 0.08);
  box-shadow: 0 18px 45px rgba(15, 23, 42, 0.08);
}

.v-theme--light .download-section__overview-card--quickstart {
  background: linear-gradient(180deg, rgba(14, 165, 233, 0.08), rgba(255, 255, 255, 0.9));
}

.v-theme--light .download-section__overview-title,
.v-theme--light .download-section__step-title,
.v-theme--light .download-section__support-name,
.v-theme--light .download-section__card-label {
  color: #0f172a;
}

.v-theme--light .download-section__overview-subtitle,
.v-theme--light .download-section__step-note,
.v-theme--light .download-section__support-note,
.v-theme--light .download-section__card-description,
.v-theme--light .download-section__command-label,
.v-theme--light .download-section__card-note {
  color: #475569;
}

.v-theme--light .download-section__step-command,
.v-theme--light .download-section__command {
  color: #164e63;
  background: rgba(241, 245, 249, 0.9);
  border-color: rgba(15, 23, 42, 0.06);
}

.v-theme--light .download-section__card {
  background: rgba(255, 255, 255, 0.8);
  border-color: rgba(0, 180, 200, 0.16);
}

@media (max-width: 960px) {
  .download-section__overview {
    grid-template-columns: 1fr;
  }

  .download-section__cards {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .download-section__cards {
    grid-template-columns: 1fr;
  }

  .download-section__step {
    grid-template-columns: 1fr;
  }

  .download-section__step-index {
    width: fit-content;
    min-width: 54px;
  }
}
</style>
