<script setup lang="ts">
import type { ClientID, RegistryPlugin } from '~/types/registry';
import { pluginCommands } from '~/utils/commands';
import { deliveryLabel, expectedDistribution, resolveDistribution } from '~/utils/registry';

const props = defineProps<{ plugin: RegistryPlugin }>();
const targets = defineModel<ClientID[]>('targets', { required: true });
const autoDetect = defineModel<boolean>('autoDetect', { required: true });
const { asset } = useSite();
const { current, expired, published } = useDirectoryStatus();
const autoOption = {
  label: 'All installed agents (recommended)',
  summary: 'All installed agents',
  description: 'Detected when you run the command',
};
const availableClients = computed(() =>
  clients.filter((client) => props.plugin.client_support.clients.includes(client.id)),
);
const targetOptions = computed(() =>
  clients.map((client) => ({
    value: client.id,
    label: client.name,
    icon: asset(`client-icons/${client.icon}`),
    disabled: !props.plugin.client_support.clients.includes(client.id),
    description: (() => {
      if (!published.value) return 'Unavailable: review data is not installation authority';
      if (expired.value) return 'Unavailable: signed Directory snapshot expired';
      const source = expectedDistribution(props.plugin, [client.id]);
      const target = source?.targets.find((item) => item.client === client.id);
      if (client.id === 'chatgpt')
        return target?.app_binding
          ? 'Verified connection; finish setup in ChatGPT'
          : 'Not available for ChatGPT';
      if (target) return deliveryLabel(target.delivery);
      return 'No active release supports this client';
    })(),
  })),
);
const commands = computed(() =>
  current.value && targets.value.length && (autoDetect.value || hasCompleteSource.value)
    ? pluginCommands(props.plugin, autoDetect.value ? undefined : targets.value)
    : undefined,
);
const resolution = computed(() => resolveDistribution(props.plugin, targets.value));
const expectedSource = computed(() => (current.value ? resolution.value.distribution : undefined));
const hasCompleteSource = computed(() => Boolean(expectedSource.value));
const selectedTargets = computed(
  () =>
    expectedSource.value?.targets.filter((target) => targets.value.includes(target.client)) ?? [],
);
const chatgptSelected = computed(
  () =>
    !autoDetect.value &&
    selectedTargets.value.some((target) => target.client === 'chatgpt' && target.app_binding),
);

function updateTargets(values: string[]) {
  const allowed = new Set(availableClients.value.map((client) => client.id));
  const next = values.filter((value): value is (typeof clients)[number]['id'] =>
    allowed.has(value as (typeof clients)[number]['id']),
  );
  if (next.length) targets.value = next;
}

function updateAutoDetect(value: boolean) {
  autoDetect.value = value;
}

watch(availableClients, (next) => {
  const allowed = new Set(next.map((client) => client.id));
  const retained = targets.value.filter((target) => allowed.has(target));
  targets.value = retained.length ? retained : next[0] ? [next[0].id] : [];
});
</script>

<template>
  <aside class="install-panel" aria-labelledby="install-title">
    <div class="install-panel__heading">
      <div>
        <p class="eyebrow">Installer</p>
        <h2 id="install-title">Use with your agent</h2>
      </div>
      <a
        class="install-panel__cli-link"
        href="https://github.com/777genius/universal-agent-plugins#quick-start"
        target="_blank"
        rel="noreferrer"
        >Native Go CLI · no Node.js</a
      >
    </div>
    <div v-if="commands" class="command-stack">
      <div class="install-command-row">
        <div class="target-select">
          <span>Agents</span
          ><AppMultiSelect
            :model-value="targets"
            :auto-selected="autoDetect"
            :auto-option="autoOption"
            label="Choose target agents"
            :options="targetOptions"
            @update:auto-selected="updateAutoDetect"
            @update:model-value="updateTargets"
          />
        </div>
        <CommandSnippet label="Add" kind="add" :command="commands.add" />
      </div>
      <CommandSnippet label="Update" kind="update" :command="commands.update" />
      <CommandSnippet label="Repair" kind="repair" :command="commands.repair" />
      <CommandSnippet label="Remove" kind="remove" :command="commands.remove" />
    </div>
    <p v-if="expired" class="install-panel__notice" role="status">
      <strong>Commands unavailable: stale Directory.</strong> This signed snapshot has expired.
      Browse its history, then return after a fresh snapshot is published.
    </p>
    <p v-else-if="!published" class="install-panel__notice" role="status">
      <strong>Commands unavailable in review preview.</strong> Unresolved data is for review only;
      production commands require a published signed Directory snapshot.
    </p>
    <p v-else-if="!autoDetect && !hasCompleteSource" class="install-panel__notice">
      <strong>Commands unavailable.</strong> {{ resolution.unavailable_reason }}
    </p>
    <p v-if="autoDetect && commands" class="install-panel__notice">
      <strong>Automatic detection.</strong> The CLI checks this plugin against installed agents,
      skips incompatible ones, and lets you confirm the targets. ChatGPT is included only when this
      plugin provides a verified connection.
    </p>
    <p v-if="chatgptSelected" class="install-panel__notice">
      <strong>One step remains in ChatGPT.</strong> Open ChatGPT, select
      {{ plugin.display_name }} in Apps or Plugins, connect it, and start a new chat. Availability
      depends on your account and workspace.
    </p>
    <p class="install-panel__notice">
      <strong>Clear outcomes.</strong> If an agent needs activation or sign-in, the CLI shows the
      exact next step. It never receives your OAuth credentials.
    </p>
    <p v-if="!autoDetect && resolution.fallback_reason && current" class="install-panel__notice">
      <strong>Expected source fallback: {{ expectedSource?.label }}.</strong>
      {{ resolution.fallback_reason }}
    </p>
    <p v-else-if="!autoDetect && !hasCompleteSource && current" class="install-panel__notice">
      <strong>No single source serves this target set.</strong> The CLI will fail before mutation
      and suggest compatible target/source combinations; it never mixes distributions across
      clients.
    </p>
    <p v-if="!plugin.built_in" class="install-panel__notice">
      <strong>Pinned direct source.</strong> Add uses the full commit pin. Update and remove use the
      installed manifest name.
    </p>
    <p v-if="plugin.client_support.resolution === 'install_time'" class="install-panel__notice">
      <strong>Checked at install time.</strong> The CLI validates the package and selected target
      before it changes managed files.
    </p>
    <p class="install-panel__footnote">
      The CLI plans all selected targets before mutation. Use <code>switch</code> to change source;
      update and repair stay on the recorded distribution.
    </p>
  </aside>
</template>
