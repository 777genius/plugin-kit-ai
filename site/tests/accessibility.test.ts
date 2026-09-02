import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, it } from 'node:test'
import { fileURLToPath } from 'node:url'

function source(relative: string): string {
  return readFileSync(fileURLToPath(new URL(relative, import.meta.url)), 'utf8')
}

describe('focused catalog accessibility contract', () => {
  it('uses a named checkbox multiselect with keyboard-native controls', () => {
    const component = source('../components/AppMultiSelect.vue')
    assert.match(component, /role="group" :aria-label="label"/)
    assert.match(component, /role="checkbox"/)
    assert.match(component, /:aria-checked=/)
    assert.match(component, /:aria-checked="!autoSelected && modelValue\.includes/)
    assert.match(component, /app-multiselect__item--selected': !autoSelected && modelValue\.includes/)
    assert.match(component, /type="button"/)
    assert.match(component, /:disabled="option\.disabled \|\| \(!autoSelected/)
  })

  it('keeps local client logos paired with accessible text and explains disabled ChatGPT', () => {
    const panel = source('../components/InstallPanel.vue')
    const multiselect = source('../components/AppMultiSelect.vue')
    assert.match(panel, /client-icons\/\$\{client\.icon\}/)
    assert.match(panel, /Not available for ChatGPT/)
    assert.match(panel, /finish setup in ChatGPT/i)
    assert.match(panel, /One step remains in ChatGPT/)
    assert.match(panel, /Availability depends on your account and workspace/)
    const detail = source('../pages/plugins/[slug].vue')
    assert.match(detail, /ChatGPT setup available/)
    assert.doesNotMatch(detail, /app key|app ID|MCP server/)
    assert.match(multiselect, /<span>\{\{ option\.label \}\}<\/span>/)
    assert.match(multiselect, /<img v-if="option\.icon"[^>]+alt=""/)
    assert.match(panel, /All installed agents \(recommended\)/)
    assert.match(multiselect, /autoOption\.label/)
    assert.match(panel, /defineModel<boolean>\('autoDetect'/)
    assert.match(detail, /v-model:auto-detect="autoDetect"/)
    assert.match(detail, /current\.value && !autoDetect\.value \? resolution\.value\.distribution/)
  })

  it('places a compact target selector above the exact generated command', () => {
    const home = source('../pages/index.vue')
    const card = source('../components/PluginCard.vue')
    const styles = source('../assets/css/main.css')
    const layout = source('../layouts/default.vue')
    assert.match(home, /class="hero-quick-start__steps"[\s\S]*AppMultiSelect[\s\S]*CommandSnippet[^>]+inline/)
    assert.match(card, /class="plugin-card__install-toggle"[\s\S]*Install[\s\S]*class="plugin-card__install-panel"[\s\S]*AppMultiSelect[\s\S]*CommandSnippet[^>]+compact/)
    assert.match(card, /:aria-expanded="installExpanded"/)
    assert.match(card, /v-if="installExpanded && canInstall"/)
    assert.match(card, /<AppMultiSelect[\s\S]*v-if="targets\.length"/)
    assert.match(styles, /\.plugin-card__install \{[^}]*grid-template-columns: minmax\(0, 1fr\)/)
    assert.match(styles, /\.plugin-card__install \.app-multiselect__trigger \{[^}]*190px/)
    assert.match(styles, /\.plugin-card \{[^}]*min-height: 300px/)
    assert.match(styles, /\.plugin-card__install-toggle \{[^}]*var\(--green\)/)
    assert.match(layout, /Pull request preview/)
  })

  it('keeps the public homepage human-readable while preserving stale safety', () => {
    const home = source('../pages/index.vue')
    const layout = source('../layouts/default.vue')
    const card = source('../components/PluginCard.vue')
    assert.match(home, /<h2>Install a plugin<\/h2>/)
    assert.match(home, /class="hero-quick-start__step"/)
    assert.doesNotMatch(home, /hero__float|Interactive installer ready|Schema validated/)
    assert.doesNotMatch(layout, /Signed Directory snapshot \{\{/)
    assert.doesNotMatch(card, /Immutable commit|Manifest/)
    assert.match(card, /stars on repo/)
    assert.match(layout, /Plugin updates are temporarily paused/)
  })

  it('chooses an installable homepage demo instead of assuming one product', () => {
    const home = source('../pages/index.vue')
    assert.match(home, /preferredDemoNames = \['chrome-devtools', 'context7', 'cloudflare-docs', 'agent-code-navigator'\]/)
    assert.match(home, /Boolean\(expectedDistribution\(item, \[client\]\)\)/)
    assert.match(home, /heroPluginName/)
    assert.match(home, /Chrome DevTools/)
    assert.match(home, /<h2>Install a plugin<\/h2>/)
    assert.doesNotMatch(home, /name === 'context7'/)
    assert.doesNotMatch(home, />Install Context7 for/)
  })

  it('gates install candidates and copy actions on a current published snapshot', () => {
    const status = source('../composables/useDirectoryStatus.ts')
    const home = source('../pages/index.vue')
    const card = source('../components/PluginCard.vue')
    const panel = source('../components/InstallPanel.vue')
    const detail = source('../pages/plugins/[slug].vue')
    assert.match(status, /registry\.data_source === 'published_snapshot' && !directoryIsExpired/)
    assert.match(home, /!current\.value.*pluginCommands/)
    assert.match(card, /current\.value \? resolution\.value\.distribution : undefined/)
    assert.match(panel, /current\.value && targets\.value\.length && \(autoDetect\.value \|\| hasCompleteSource\.value\) \? pluginCommands/)
    assert.match(detail, /current\.value && !autoDetect\.value \? resolution\.value\.distribution : undefined/)
    for (const view of [home, card, panel, detail]) assert.match(view, /review (?:data|preview)/i)
  })

  it('exposes both Add a plugin pull-request actions', () => {
    const catalog = source('../components/PluginCatalog.vue')
    assert.equal([...catalog.matchAll(/registry\/README\.md#submit-an-external-package/g)].length, 2)
    assert.match(catalog, /class="catalog-heading-row"/)
    assert.match(catalog, /class="catalog-add-button"[^>]+aria-label="Add a plugin"[^>]+aria-describedby="catalog-add-tooltip"/)
    assert.match(catalog, /role="tooltip">Add a plugin/)
    assert.match(catalog, /Add a plugin by pull request/)
  })

  it('renders contributor copy as text under a restrictive static CSP', () => {
    const config = source('../nuxt.config.ts')
    const components = [source('../components/PluginCard.vue'), source('../pages/plugins/[slug].vue')].join('\n')
    assert.match(config, /contentSecurityPolicy = "default-src 'self'/)
    assert.match(config, /object-src 'none'/)
    assert.doesNotMatch(components, /v-html/)
  })

  it('renders package evidence once with its exact immutable artifact identity', () => {
    const detail = source('../pages/plugins/[slug].vue')
    assert.equal([...detail.matchAll(/v-for="item in installCandidate\.package_evidence"/g)].length, 1)
    assert.match(detail, /item\.artifact\.repository.*item\.artifact\.revision.*item\.artifact\.path/s)
    assert.match(detail, /item\.artifact\.digest/)
    assert.match(detail, /item\.package_tree_digest/)
    assert.match(detail, /item\.artifact\.url/)
  })

  it('binds every target-dependent install field to the exact resolved release', () => {
    const card = source('../components/PluginCard.vue')
    const detail = source('../pages/plugins/[slug].vue')
    assert.match(card, /selectedDistribution\.components/)
    assert.match(card, /githubSourceUrl\(plugin, selectedDistribution\)/)
    assert.match(card, /authenticationLabel\(resolution\.value\.distribution, targets\.value, props\.plugin\.authentication\)/)
    assert.doesNotMatch(card, /plugin\.(?:version|components|evidence|package_evidence)/)
    for (const field of ['version', 'components', 'source', 'package_evidence', 'evidence']) {
      assert.match(detail, new RegExp(`installCandidate\\.${field}`))
    }
    assert.match(detail, /v-model:targets="targets"/)
    assert.match(detail, /Product release history/)
    assert.match(detail, /authenticationLabel\(installCandidate\.value, targets\.value, plugin\.authentication\)/)
    assert.match(detail, /targetAuthenticationLabel\(target\.authentication\)/)
    assert.match(detail, /not the selected install candidate/)
    assert.doesNotMatch(detail, /plugin\.(?:version|components|evidence|package_evidence)/)
  })

  it('does not invent fallback plugin avatars and keeps reviewed status in a corner ribbon', () => {
    const site = source('../composables/useSite.ts')
    const card = source('../components/PluginCard.vue')
    assert.doesNotMatch(site, /path \? asset\(path\) : asset\('logo\.svg'\)/)
    assert.match(card, /v-if="iconURL" class="plugin-card__icon"/)
    assert.match(card, /class="plugin-card__ribbon"/)
    assert.doesNotMatch(card, /Package format checked/)
  })
})
