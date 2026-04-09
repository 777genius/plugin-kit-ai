import context7Logo from '~/assets/images/plugin-logos/context7.png';
import firebaseLogo from '~/assets/images/plugin-logos/firebase.svg';
import githubLogo from '~/assets/images/plugin-logos/github.svg';
import gitlabLogo from '~/assets/images/plugin-logos/gitlab.svg';
import greptileLogo from '~/assets/images/plugin-logos/greptile.svg';
import linearLogo from '~/assets/images/plugin-logos/linear.svg';
import notionLogo from '~/assets/images/plugin-logos/notion.png';
import sentryLogo from '~/assets/images/plugin-logos/sentry.svg';
import slackLogo from '~/assets/images/plugin-logos/slack.svg';
import stripeLogo from '~/assets/images/plugin-logos/stripe.svg';
import supabaseLogo from '~/assets/images/plugin-logos/supabase.svg';
import vercelLogo from '~/assets/images/plugin-logos/vercel.svg';
import type { PluginLogoSurface } from '~/types/content';

interface PluginLogoDefinition {
  src: string;
  surface?: PluginLogoSurface;
}

const pluginLogos: Record<string, PluginLogoDefinition> = {
  'context7.svg': { src: context7Logo },
  'firebase.svg': { src: firebaseLogo },
  'github.svg': { src: githubLogo, surface: 'light' },
  'gitlab.svg': { src: gitlabLogo },
  'greptile.svg': { src: greptileLogo },
  'linear.svg': { src: linearLogo },
  'notion.svg': { src: notionLogo },
  'sentry.svg': { src: sentryLogo },
  'slack.svg': { src: slackLogo },
  'stripe.svg': { src: stripeLogo },
  'supabase.svg': { src: supabaseLogo },
  'vercel.svg': { src: vercelLogo },
};

export const resolvePluginLogo = (logoSrc: string): PluginLogoDefinition => {
  return pluginLogos[logoSrc] ?? { src: logoSrc, surface: 'default' };
};
