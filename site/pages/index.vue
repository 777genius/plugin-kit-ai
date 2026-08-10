<script setup lang="ts">
const registry = useRegistry()
const { repositoryUrl } = useSite()
const builtInCount = computed(() => registry.plugins.filter(plugin => plugin.built_in).length)
const externalCount = computed(() => registry.plugins.length - builtInCount.value)
const description = 'Discover and manage Agent Plugins 1.0 across Codex, ChatGPT, Cursor, GitHub Copilot CLI, VS Code, and Kiro with one community CLI.'

useSeoMeta({
  title: 'A clearer way to use Agent Plugins',
  description,
  ogTitle: 'Universal Agent Plugins',
  ogDescription: description,
  ogType: 'website',
  twitterCard: 'summary',
})
useHead({ link: [{ rel: 'canonical', href: `${useRuntimeConfig().public.siteUrl}/` }] })
</script>

<template>
  <div>
    <section class="hero container">
      <div class="hero__copy">
        <div class="hero__badge"><span /> Community directory for Agent Plugins 1.0</div>
        <h1>One command.<br /><em>Every supported client.</em></h1>
        <p class="hero__lead">Add, update, or remove portable agent abilities across the tools you already use. Choose a plugin, choose a target, and let the community CLI handle the client-specific layout.</p>
        <div class="hero__actions">
          <NuxtLink class="button button--primary" to="/plugins">Explore {{ builtInCount }} plugins <span aria-hidden="true">→</span></NuxtLink>
          <a class="button button--secondary" :href="`${repositoryUrl}/blob/main/registry/README.md#submit-an-external-package`" target="_blank" rel="noreferrer">Submit a plugin</a>
        </div>
        <p class="hero__fine-print">Open source · No tracking · Review before enabling</p>
      </div>
      <div class="hero__demo">
        <div class="hero__window">
          <div class="hero__window-top"><span /><span /><span /><b>Quick start</b></div>
          <div class="hero__window-body">
            <p>Install Context7 for Cursor</p>
            <CommandSnippet command="npx universal-agent-plugins add context7 --target cursor" />
            <div class="hero__success"><span>✓</span><div><strong>Ready for Cursor</strong><small>Start a new session, then ask for current docs.</small></div></div>
          </div>
        </div>
        <div class="hero__float hero__float--schema"><span>✓</span> Schema validated</div>
        <div class="hero__float hero__float--portable">1.0<br /><small>portable</small></div>
      </div>
    </section>

    <section class="client-section container" aria-labelledby="clients-title">
      <p id="clients-title">Select a target, keep your workflow</p>
      <ClientStrip />
      <p class="client-section__note">Compatibility describes supported package components—not identical marketplaces or proof that every runtime and OAuth path was tested.</p>
    </section>

    <section id="how-it-works" class="how container" aria-labelledby="how-title">
      <div class="section-heading section-heading--center">
        <p class="eyebrow">A small, explicit workflow</p>
        <h2 id="how-title">From directory to client in three steps</h2>
      </div>
      <ol class="step-grid">
        <li><span>01</span><div><h3>Pick a plugin</h3><p>Review its source, components, permissions, and validation status.</p></div></li>
        <li><span>02</span><div><h3>Choose a target</h3><p>Generate the exact command for Codex, ChatGPT, Cursor, Copilot, VS Code, or Kiro.</p></div></li>
        <li><span>03</span><div><h3>Stay in control</h3><p>Use the same CLI to update or remove it. Follow any client activation or OAuth prompt.</p></div></li>
      </ol>
    </section>

    <div class="container catalog-wrap">
      <PluginCatalog
        :plugins="registry.plugins"
        :heading="`${registry.plugins.length} plugins, one generated index`"
        :intro="externalCount ? `${builtInCount} reviewed built-ins and ${externalCount} pinned external ${externalCount === 1 ? 'entry' : 'entries'}.` : `${builtInCount} reviewed built-ins today, with commit-pinned external submissions supported next.`"
      />
    </div>

    <section class="validation-section container" aria-labelledby="validation-title">
      <div>
        <p class="eyebrow">Read status precisely</p>
        <h2 id="validation-title">Validated structure is not a runtime promise.</h2>
      </div>
      <div class="validation-section__cards">
        <article><span class="validation-badge"><span>✓</span> Schema validated</span><p>The manifest and declared components passed the repository’s structural checks.</p></article>
        <article><span class="runtime-badge">◇ Runtime evidence</span><p>Runtime, authentication, and OAuth coverage varies by plugin and client. Check the linked evidence before relying on a path.</p></article>
      </div>
    </section>

    <section class="submit-cta container">
      <div><p class="eyebrow">Built in the open</p><h2>Have a useful Agent Plugin?</h2><p>Submit a schema-valid package with a reviewable source. External entries stay pinned to an immutable commit.</p></div>
      <a class="button button--primary" :href="`${repositoryUrl}/blob/main/registry/README.md#submit-an-external-package`" target="_blank" rel="noreferrer">Submit a plugin <span aria-hidden="true">↗</span></a>
    </section>
  </div>
</template>
