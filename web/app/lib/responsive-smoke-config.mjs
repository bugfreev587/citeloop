export const VIEWPORTS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "tablet-landscape", width: 1024, height: 768 },
  { name: "tablet-portrait", width: 768, height: 1024 },
  { name: "mobile", width: 390, height: 844 },
];

export const CORE_PAGE_LABELS = [
  "Home",
  "Dashboard",
  "Knowledge",
  "Topics",
  "Review",
  "Publishing",
  "SEO",
  "Runs",
  "Settings",
];

export function corePagePaths(projectId) {
  return [
    { label: "Home", path: "/" },
    { label: "Dashboard", path: `/projects/${projectId}` },
    { label: "Knowledge", path: `/projects/${projectId}/knowledge` },
    { label: "Topics", path: `/projects/${projectId}/topics` },
    { label: "Review", path: `/projects/${projectId}/review` },
    { label: "Publishing", path: `/projects/${projectId}/publishing` },
    { label: "SEO", path: `/projects/${projectId}/seo` },
    { label: "Runs", path: `/projects/${projectId}/runs` },
    { label: "Settings", path: `/projects/${projectId}/settings` },
  ];
}

export function assessResponsiveMetrics(metrics) {
  const failures = [];
  if (metrics.targetPath && metrics.landedPath && metrics.targetPath !== metrics.landedPath) {
    failures.push(`${metrics.page} ${metrics.viewport} landed on ${metrics.landedPath} instead of ${metrics.targetPath}`);
  }
  if (metrics.documentWidth > metrics.viewportWidth + 1) {
    failures.push(
      `${metrics.page} ${metrics.viewport} has horizontal overflow: document ${metrics.documentWidth}px > viewport ${metrics.viewportWidth}px`,
    );
  }
  if (metrics.protrudingElements.length > 0) {
    failures.push(
      `${metrics.page} ${metrics.viewport} has clipped primary content: ${metrics.protrudingElements
        .map((item) => item.label)
        .join(", ")}`,
    );
  }
  if (metrics.overlappingControls.length > 0) {
    failures.push(
      `${metrics.page} ${metrics.viewport} has overlapping controls: ${metrics.overlappingControls
        .map((item) => `${item.a} overlaps ${item.b}`)
        .join("; ")}`,
    );
  }
  if (metrics.disabledControlsWithoutExplanation.length > 0) {
    failures.push(
      `${metrics.page} ${metrics.viewport} has disabled controls without explanation: ${metrics.disabledControlsWithoutExplanation.join(", ")}`,
    );
  }
  return failures;
}
