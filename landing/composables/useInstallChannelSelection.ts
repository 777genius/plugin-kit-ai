import type { ComputedRef } from 'vue';
import type { InstallChannel } from '~/types/content';
import {
  normalizeInstallPlatform,
  recommendedInstallChannelId,
  type InstallPlatform,
} from '~/utils/installPlatform';

export function useInstallChannelSelection(channels: ComputedRef<InstallChannel[]>) {
  const selectedInstallChannelId = ref<string | null>(null);
  const detectedInstallPlatform = ref<InstallPlatform | null>(null);
  const manuallySelected = ref(false);

  const recommendedChannelId = computed(() => {
    const detectedRecommendation = detectedInstallPlatform.value
      ? recommendedInstallChannelId(detectedInstallPlatform.value)
      : null;
    if (
      detectedRecommendation &&
      channels.value.some((channel) => channel.id === detectedRecommendation)
    ) {
      return detectedRecommendation;
    }

    return (
      channels.value.find((channel) => channel.recommended)?.id ?? channels.value[0]?.id ?? null
    );
  });

  watchEffect(() => {
    const selectionStillExists = channels.value.some(
      (channel) => channel.id === selectedInstallChannelId.value,
    );
    if (!selectionStillExists || !manuallySelected.value) {
      selectedInstallChannelId.value = recommendedChannelId.value;
    }
  });

  onMounted(async () => {
    try {
      const { default: Bowser } = await import('bowser');
      detectedInstallPlatform.value = normalizeInstallPlatform(
        Bowser.getParser(window.navigator.userAgent).getOSName(true),
      );
    } catch {
      detectedInstallPlatform.value = null;
    }
  });

  function selectInstallChannel(channelId: string) {
    if (!channels.value.some((channel) => channel.id === channelId)) {
      return;
    }
    manuallySelected.value = true;
    selectedInstallChannelId.value = channelId;
  }

  return {
    detectedInstallPlatform,
    recommendedChannelId,
    selectedInstallChannelId,
    selectInstallChannel,
  };
}
