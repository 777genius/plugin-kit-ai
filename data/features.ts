import {
  mdiChartTimelineVariant,
  mdiCodeBracesBox,
  mdiMessageTextOutline,
  mdiOpenSourceInitiative,
  mdiRobotOutline,
  mdiViewDashboardOutline
} from "@mdi/js";

export const features = [
  { id: "oneRepo", icon: mdiRobotOutline, accent: "#00f0ff" },
  { id: "growOutputs", icon: mdiViewDashboardOutline, accent: "#ff00ff" },
  { id: "validate", icon: mdiCodeBracesBox, accent: "#39ff14" },
  { id: "clearBoundary", icon: mdiMessageTextOutline, accent: "#ffd700" },
  { id: "releasePath", icon: mdiChartTimelineVariant, accent: "#00f0ff" },
  { id: "openSource", icon: mdiOpenSourceInitiative, accent: "#ff00ff" }
] as const;
