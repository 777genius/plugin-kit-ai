<script setup lang="ts">
import type { RegistryPlugin } from '~/types/registry'
import { pluginCommands } from '~/utils/commands'

const props = defineProps<{ plugin: RegistryPlugin }>()
const target = ref<(typeof clients)[number]['id']>('cursor')
const commands = computed(() => pluginCommands(props.plugin, target.value))
</script>

<template>
  <aside class="install-panel" aria-labelledby="install-title">
    <div class="install-panel__heading">
      <div><p class="eyebrow">Installer</p><h2 id="install-title">Use with your client</h2></div>
      <span>Node.js 22+</span>
    </div>
    <label class="target-select">
      Target client
      <select v-model="target">
        <option v-for="client in clients" :key="client.id" :value="client.id">{{ client.name }}</option>
      </select>
    </label>
    <div class="command-stack">
      <CommandSnippet label="Add" :command="commands.add" />
      <CommandSnippet label="Update" :command="commands.update" />
      <CommandSnippet label="Remove" :command="commands.remove" />
    </div>
    <p v-if="!plugin.built_in" class="install-panel__notice"><strong>Pinned external source.</strong> The full commit-pinned source above is required; external plugins do not resolve by short name.</p>
    <p class="install-panel__footnote">The CLI adapts the package for the target. Client UI activation, permissions, or OAuth may still require a separate confirmation.</p>
  </aside>
</template>
