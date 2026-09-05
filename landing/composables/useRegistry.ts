import type { RegistryIndex } from '~/types/registry';
import { BrowserDiscoveryCache, discoveryPlugin, loadDiscovery } from '~/utils/discovery';
import { applySecurityAssessment, loadSecurity } from '~/utils/security';

export function useRegistry(): RegistryIndex {
  const config = useRuntimeConfig();
  const registry = useState<RegistryIndex>('registry-index', () =>
    structuredClone(config.public.registryIndex as unknown as RegistryIndex),
  );
  const started = useState('discovery-started', () => false);
  const status = useDiscoveryStatus();

  if (import.meta.client) {
    onMounted(async () => {
      if (started.value) return;
      started.value = true;
      status.value = { state: 'loading', count: 0 };
      const baseURL = String(config.public.baseURL).replace(/\/?$/, '/');
      const origin = new URL(`${baseURL}discovery/`, location.origin);
      const security = loadSecurity({
        origin: new URL(`${baseURL}security/`, location.origin),
        trust: {
          keyID: String(config.public.discoveryKeyID),
          publicKeyBase64: String(config.public.discoveryPublicKey),
        },
      }).catch(() => undefined);
      try {
        const bundle = await loadDiscovery({
          origin,
          trust: {
            keyID: String(config.public.discoveryKeyID),
            publicKeyBase64: String(config.public.discoveryPublicKey),
          },
          cache: new BrowserDiscoveryCache(origin),
        });
        await waitForCatalogInteractionToFinish();
        const reviewed = registry.value.plugins.filter(
          (plugin) => plugin.trust_state !== 'conformant_unreviewed',
        );
        const discovered = bundle.search.records.map((record) =>
          discoveryPlugin(record, bundle.snapshot),
        );
        registry.value.plugins = [...reviewed, ...discovered];
        status.value = {
          state: bundle.source === 'remote' ? 'current' : 'cached',
          count: discovered.length,
          sequence: bundle.snapshot.sequence,
          generatedAt: bundle.snapshot.generated_at,
        };
        const securityBundle = await security;
        if (securityBundle) {
          await waitForCatalogInteractionToFinish();
          registry.value.plugins = registry.value.plugins.map((plugin) =>
            applySecurityAssessment(plugin, securityBundle.snapshot),
          );
        }
      } catch (error) {
        registry.value.plugins = registry.value.plugins.filter(
          (plugin) => plugin.trust_state !== 'conformant_unreviewed',
        );
        status.value = {
          state:
            error instanceof Error && /stale|expired/i.test(error.message)
              ? 'stale'
              : 'unavailable',
          count: 0,
          message: error instanceof Error ? error.message : 'Signed Discovery Index is unavailable',
        };
      }
    });
  }

  return registry.value;
}

function waitForCatalogInteractionToFinish(): Promise<void> {
  const isCatalogInteraction = () =>
    document.activeElement instanceof Element &&
    Boolean(
      document.activeElement.closest(
        '.catalog, .app-combobox__content, .app-select__content, .app-multiselect__content',
      ),
    );
  if (!isCatalogInteraction()) return Promise.resolve();
  return new Promise((resolve) => {
    const observe = () =>
      queueMicrotask(() => {
        if (isCatalogInteraction()) return;
        document.removeEventListener('focusin', observe);
        document.removeEventListener('focusout', observe);
        resolve();
      });
    document.addEventListener('focusin', observe);
    document.addEventListener('focusout', observe);
  });
}
