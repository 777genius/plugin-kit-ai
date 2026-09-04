<script setup lang="ts">
const { asset } = useSite();
const featuredClientIDs = new Set(['codex', 'cursor', 'claude', 'gemini', 'opencode', 'windsurf']);
const featuredClients = clients.filter((client) => featuredClientIDs.has(client.id));
</script>

<template>
  <div class="hero-agent-field" aria-hidden="true">
    <span class="hero-agent-field__orbit hero-agent-field__orbit--outer" />
    <span class="hero-agent-field__orbit hero-agent-field__orbit--inner" />
    <span class="hero-agent-field__hub">
      <img :src="asset('icon.svg')" alt="" width="34" height="34" >
    </span>
    <span
      v-for="(client, index) in featuredClients"
      :key="client.id"
      class="hero-agent-field__node"
      :class="`hero-agent-field__node--${index + 1}`"
    >
      <img :src="asset(`client-icons/${client.icon}`)" alt="" width="22" height="22" >
    </span>
    <span class="hero-agent-field__signal hero-agent-field__signal--1" />
    <span class="hero-agent-field__signal hero-agent-field__signal--2" />
    <span class="hero-agent-field__signal hero-agent-field__signal--3" />
  </div>
</template>
