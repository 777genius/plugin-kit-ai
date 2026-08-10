<!--
  Adapted from plugin-kit-ai landing/components/PageBackground.vue (MIT).
  See site/NOTICE.md for the complete attribution.
-->
<template>
  <div class="page-background" aria-hidden="true">
    <div class="page-background__grid" />
    <div class="page-background__orb page-background__orb--violet" />
    <div class="page-background__orb page-background__orb--cyan" />
    <div class="page-background__orb page-background__orb--coral" />
  </div>
</template>

<style scoped>
.page-background {
  position: fixed;
  inset: 0;
  z-index: -1;
  overflow: hidden;
  pointer-events: none;
}

.page-background__grid {
  position: absolute;
  inset: 0;
  background-image: linear-gradient(var(--grid) 1px, transparent 1px), linear-gradient(90deg, var(--grid) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: linear-gradient(to bottom, black, transparent 82%);
}

.page-background__orb {
  position: absolute;
  width: 40rem;
  aspect-ratio: 1;
  border-radius: 50%;
  filter: blur(130px);
  opacity: .13;
  animation: drift 18s ease-in-out infinite alternate;
}

.page-background__orb--violet { background: #7655ff; top: -22rem; left: 15%; }
.page-background__orb--cyan { background: #16b9da; top: 32rem; right: -22rem; animation-delay: -5s; }
.page-background__orb--coral { background: #f26e5f; top: 92rem; left: -26rem; animation-delay: -10s; }

@keyframes drift { to { transform: translate3d(2rem, 3rem, 0); } }
@media (prefers-reduced-motion: reduce) { .page-background__orb { animation: none; } }
</style>
