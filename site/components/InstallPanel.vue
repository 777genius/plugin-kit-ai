<script setup lang="ts">
import type { RegistryPlugin } from '~/types/registry'
import { pluginCommands } from '~/utils/commands'

const props = defineProps<{ plugin: RegistryPlugin }>()
const { asset } = useSite()
const availableClients = computed(() => clients.filter(client => props.plugin.client_support.clients.includes(client.id)))
const initialTarget = availableClients.value.find(client => client.id === 'cursor')?.id ?? availableClients.value[0]!.id
const targets = ref<(typeof clients)[number]['id'][]>([initialTarget])
const targetOptions = computed(() => availableClients.value.map(client => ({
  value: client.id,
  label: client.name,
  icon: asset(`client-icons/${client.icon}`),
})))
const commands = computed(() => pluginCommands(props.plugin, targets.value))

function updateTargets(values: string[]) {
  const allowed = new Set(availableClients.value.map(client => client.id))
  const next = values.filter((value): value is (typeof clients)[number]['id'] => allowed.has(value as (typeof clients)[number]['id']))
  if (next.length) targets.value = next
}

watch(availableClients, (next) => {
  const allowed = new Set(next.map(client => client.id))
  const retained = targets.value.filter(target => allowed.has(target))
  targets.value = retained.length ? retained : [next[0]!.id]
})
</script>

<template>
  <aside class="install-panel" aria-labelledby="install-title">
    <div class="install-panel__heading">
      <div><p class="eyebrow">Installer</p><h2 id="install-title">Use with your agent</h2></div>
      <span>Node.js 22+</span>
    </div>
    <div class="target-select">
      <span>Target agents</span>
      <AppMultiSelect :model-value="targets" label="Choose target agents" :options="targetOptions" @update:model-value="updateTargets" />
    </div>
    <div class="command-stack">
      <CommandSnippet label="Add" kind="add" :command="commands.add" />
      <CommandSnippet label="Update" kind="update" :command="commands.update" />
      <CommandSnippet label="Remove" kind="remove" :command="commands.remove" />
    </div>
    <p v-if="!plugin.built_in" class="install-panel__notice"><strong>Pinned external source.</strong> Add uses the full commit pin. Update and remove use the installed manifest name; the directory provides no alias or automatic latest-version lookup.</p>
    <p v-if="plugin.client_support.resolution === 'install_time'" class="install-panel__notice"><strong>Checked at install time.</strong> The CLI validates the package and selected target before it changes managed files.</p>
    <p class="install-panel__footnote">Built-in targets come from the pinned compatibility catalog. Agent UI activation, permissions, runtime behavior, or OAuth may still require separate confirmation.</p>
  </aside>
</template>
