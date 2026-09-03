const DEFAULT_CLI_INVOCATION = 'universal-agent-plugins';

export function applyCliInvocation(command: string, invocation?: string | null): string {
  const normalizedInvocation = invocation?.trim() || DEFAULT_CLI_INVOCATION;
  return command.replace(/^plugin-kit-ai\b/gm, normalizedInvocation);
}

export function getCliInvocation(invocation?: string | null): string {
  return invocation?.trim() || DEFAULT_CLI_INVOCATION;
}
