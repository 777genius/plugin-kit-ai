import type { RegistryPlugin } from '../types/registry';
import type { SecurityFinding } from '../types/security';

export type SecurityFindingAudience = 'installer' | 'maintainer';

export interface SecurityFindingGroups {
  installer: SecurityFinding[];
  maintainer: SecurityFinding[];
  hidden: number;
}

type SecurityAssessment = NonNullable<RegistryPlugin['security']>;

// These findings describe repository automation. The workflows are not copied
// to an agent or executed during installation, so present them as maintainer
// hardening notes without hiding them from the public report.
const maintainerFindingCodes = new Set(['SEC324', 'SEC325', 'SEC326', 'SEC327', 'SEC328']);

export function securityFindingAudience(finding: SecurityFinding): SecurityFindingAudience {
  return maintainerFindingCodes.has(finding.code) ? 'maintainer' : 'installer';
}

export function groupSecurityFindings(assessment: SecurityAssessment): SecurityFindingGroups {
  const groups: SecurityFindingGroups = { installer: [], maintainer: [], hidden: 0 };
  for (const finding of assessment.findings) {
    groups[securityFindingAudience(finding)].push(finding);
  }
  groups.hidden = Math.max(0, assessment.counts.total - assessment.findings.length);
  return groups;
}

export function securityAssessmentLabel(assessment: SecurityAssessment): string {
  if (assessment.counts.blocking > 0) {
    return `Automated review: ${formatCount(assessment.counts.blocking, 'blocking finding')}`;
  }
  if (assessment.counts.warnings > 0) {
    return `Automated review: ${formatCount(assessment.counts.warnings, 'note')}`;
  }
  return 'Automated review: no blocking findings';
}

export function securityAssessmentHeading(assessment: SecurityAssessment): string {
  const groups = groupSecurityFindings(assessment);
  if (assessment.counts.blocking > 0) return 'Review before installing';
  if (assessment.counts.warnings === 0) return 'No blocking findings detected';
  if (groups.installer.length === 0 && groups.hidden === 0) return 'Repository maintenance notes';
  return 'Automated review notes';
}

export function securityAssessmentTooltip(plugin: RegistryPlugin): string {
  const assessment = plugin.security;
  if (!assessment) return '';
  const groups = groupSecurityFindings(assessment);
  const revision = shortRevision(plugin.source.revision);
  const lines = [
    securityAssessmentLabel(assessment),
    `Checked indexed revision ${revision} with LintAI ${assessment.scanner.version}.`,
  ];
  const preview = [...groups.installer, ...groups.maintainer].slice(0, 2);
  lines.push(...preview.map((finding) => `${finding.code}: ${finding.message}`));
  const remaining = assessment.counts.total - preview.length;
  if (remaining > 0) lines.push(`+${remaining} more in the full review.`);
  lines.push('This result applies only to the checked files and is not a guarantee of safety.');
  return lines.join('\n');
}

export function shortRevision(revision: string | null): string {
  return revision?.slice(0, 12) || 'unknown';
}

export function formatSecurityDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('en', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(date);
}

export function securityFindingLocation(finding: SecurityFinding): string {
  const path = finding.path.trim();
  if (!path) return '';
  return finding.line ? `${path}:${finding.line}` : path;
}

function formatCount(count: number, singular: string): string {
  return `${count} ${singular}${count === 1 ? '' : 's'}`;
}
