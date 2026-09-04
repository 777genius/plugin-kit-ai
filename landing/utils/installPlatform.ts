export type InstallPlatform = 'macos' | 'windows' | 'linux' | 'other';

export function normalizeInstallPlatform(osName?: string | null): InstallPlatform {
  const normalized = osName?.trim().toLowerCase() ?? '';

  if (normalized.includes('mac') || normalized.includes('os x')) {
    return 'macos';
  }
  if (normalized.includes('windows')) {
    return 'windows';
  }
  if (normalized.includes('linux')) {
    return 'linux';
  }

  return 'other';
}

export function recommendedInstallChannelId(platform: InstallPlatform): string {
  switch (platform) {
    case 'macos':
      return 'brew';
    case 'windows':
      return 'powershell';
    case 'linux':
      return 'script';
    default:
      return 'npm';
  }
}
