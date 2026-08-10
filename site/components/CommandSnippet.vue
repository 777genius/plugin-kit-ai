<!--
  Adapted from plugin-kit-ai landing/components/shared/CommandSnippetCard.vue (MIT).
  See site/NOTICE.md for the complete attribution.
-->
<script setup lang="ts">
const props = withDefaults(defineProps<{ command: string, label?: string }>(), { label: 'Terminal' })
const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

onBeforeUnmount(() => { if (timer) clearTimeout(timer) })

async function copyCommand() {
  try {
    await navigator.clipboard.writeText(props.command)
  } catch {
    const field = document.createElement('textarea')
    field.value = props.command
    field.style.position = 'fixed'
    field.style.opacity = '0'
    document.body.appendChild(field)
    field.select()
    document.execCommand('copy')
    field.remove()
  }
  copied.value = true
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { copied.value = false }, 1600)
}
</script>

<template>
  <div class="command-snippet">
    <div class="command-snippet__header">
      <span><i />{{ label }}</span>
      <button type="button" :aria-label="copied ? 'Command copied' : 'Copy command'" @click="copyCommand">
        {{ copied ? 'Copied' : 'Copy' }}
      </button>
    </div>
    <pre><code>{{ command }}</code></pre>
    <span class="sr-only" role="status" aria-live="polite">{{ copied ? 'Command copied to clipboard' : '' }}</span>
  </div>
</template>
