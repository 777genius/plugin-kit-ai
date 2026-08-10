import { readFileSync } from 'node:fs'
import type { RegistryIndex } from '../types/registry'
import { parseRegistryIndex } from '../utils/registry'

export function loadRegistryIndex(path: string): RegistryIndex {
  let sourceText: string
  try {
    sourceText = readFileSync(path, 'utf8')
  } catch (error) {
    throw new Error(`Unable to read registry index at ${path}: ${String(error)}`)
  }
  try {
    return parseRegistryIndex(JSON.parse(sourceText) as unknown)
  } catch (error) {
    throw new Error(`Invalid registry index at ${path}: ${String(error)}`)
  }
}
