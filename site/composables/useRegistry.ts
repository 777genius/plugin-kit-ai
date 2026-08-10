import type { RegistryIndex } from '~/types/registry'

export function useRegistry(): RegistryIndex {
  const config = useRuntimeConfig()
  return config.public.registryIndex as unknown as RegistryIndex
}
