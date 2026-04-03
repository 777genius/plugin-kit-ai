import type { LocaleCode } from "~/data/i18n";

export interface FeatureItem {
  id: string;
  title: string;
  description: string;
}

export interface PluginCard {
  id: string;
  eyebrow: string;
  title: string;
  description: string;
  status: string;
  badges: string[];
  previewLines: string[];
}

export interface FaqItem {
  id: string;
  question: string;
  answer: string;
}

export interface HeroContent {
  title: string;
  subtitle: string;
}

export interface DownloadContent {
  title: string;
  note: string;
}

export interface InstallChannel {
  id: string;
  title: string;
  description: string;
  href: string;
  note: string;
  command?: string;
  recommended?: boolean;
}

export interface QuickstartStep {
  id: string;
  title: string;
  command: string;
  note: string;
}

export interface SupportLane {
  id: string;
  name: string;
  status: string;
  note: string;
}

export interface ComparisonCell {
  status: "yes" | "partial" | "no";
  note: string;
}

export interface ComparisonRow {
  id: string;
  feature: string;
  pluginKitAi: ComparisonCell;
  manual: ComparisonCell;
  duplicated: ComparisonCell;
  scripts: ComparisonCell;
}

export interface LandingContent {
  hero: HeroContent;
  features: FeatureItem[];
  plugins: PluginCard[];
  comparisonRows: ComparisonRow[];
  faq: FaqItem[];
  download: DownloadContent;
  installChannels: InstallChannel[];
  quickstartSteps: QuickstartStep[];
  supportLanes: SupportLane[];
}

export type LocalizedContent = Record<LocaleCode, LandingContent>;
