<script setup lang="ts">
const items = [
  [
    'Does one command install into every agent?',
    'By default the CLI detects installed compatible agents and asks you to confirm. You can also select one or many agents explicitly.',
  ],
  [
    'What happens when an agent needs sign-in?',
    'The CLI prepares the package and shows the exact activation or sign-in step. It never receives your OAuth password.',
  ],
  [
    'Can I install a plugin outside this directory?',
    'Yes. Use a pinned GitHub source that contains a valid Agent Plugins 1.0 plugin.json package.',
  ],
  [
    'Are all discovered plugins reviewed?',
    'No. Reviewed entries and automatically discovered community packages are labeled separately so you can choose the trust level you need.',
  ],
  [
    'Can I undo an installation?',
    'Yes. The same CLI records managed state and supports inspect, update, repair, source switching, and removal.',
  ],
] as const;
</script>

<template>
  <section id="faq" class="registry-faq container" aria-labelledby="faq-title">
    <div class="section-heading section-heading--center">
      <p class="eyebrow">FAQ</p>
      <h2 id="faq-title">Common questions</h2>
    </div>
    <div class="registry-faq__list">
      <details v-for="item in items" :key="item[0]">
        <summary>{{ item[0] }}<span aria-hidden="true">+</span></summary>
        <p>{{ item[1] }}</p>
      </details>
    </div>
  </section>
</template>
