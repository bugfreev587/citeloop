import test from "node:test";
import assert from "node:assert/strict";

import {
  CORE_PAGE_LABELS,
  VIEWPORTS,
  assessResponsiveMetrics,
  corePagePaths,
} from "./responsive-smoke-config.mjs";

test("responsive smoke matrix covers required viewports and core pages", () => {
  assert.deepEqual(VIEWPORTS, [
    { name: "desktop", width: 1440, height: 900 },
    { name: "tablet-landscape", width: 1024, height: 768 },
    { name: "tablet-portrait", width: 768, height: 1024 },
    { name: "mobile", width: 390, height: 844 },
  ]);

  assert.deepEqual(CORE_PAGE_LABELS, [
    "Home",
    "Dashboard",
    "Knowledge",
    "Topics",
    "Review",
    "Publishing",
    "SEO",
    "Runs",
    "Settings",
  ]);

  assert.deepEqual(corePagePaths("project-123"), [
    { label: "Home", path: "/" },
    { label: "Dashboard", path: "/projects/project-123" },
    { label: "Knowledge", path: "/projects/project-123/knowledge" },
    { label: "Topics", path: "/projects/project-123/topics" },
    { label: "Review", path: "/projects/project-123/review" },
    { label: "Publishing", path: "/projects/project-123/publishing" },
    { label: "SEO", path: "/projects/project-123/seo" },
    { label: "Runs", path: "/projects/project-123/runs" },
    { label: "Settings", path: "/projects/project-123/settings" },
  ]);
});

test("responsive metric assessment fails horizontal overflow and overlapping controls", () => {
  assert.deepEqual(
    assessResponsiveMetrics({
      page: "Review",
      viewport: "mobile",
      documentWidth: 410,
      viewportWidth: 390,
      protrudingElements: [],
      overlappingControls: [],
      disabledControlsWithoutExplanation: [],
    }),
    ["Review mobile has horizontal overflow: document 410px > viewport 390px"],
  );

  assert.deepEqual(
    assessResponsiveMetrics({
      page: "Review",
      viewport: "desktop",
      documentWidth: 1440,
      viewportWidth: 1440,
      protrudingElements: [],
      overlappingControls: [{ a: "Approve", b: "Reject" }],
      disabledControlsWithoutExplanation: ["AI fix"],
    }),
    [
      "Review desktop has overlapping controls: Approve overlaps Reject",
      "Review desktop has disabled controls without explanation: AI fix",
    ],
  );
});

test("responsive metric assessment fails auth redirects away from target project pages", () => {
  assert.deepEqual(
    assessResponsiveMetrics({
      page: "Review",
      viewport: "desktop",
      documentWidth: 1440,
      viewportWidth: 1440,
      targetPath: "/projects/project-123/review",
      landedPath: "/sign-in",
      protrudingElements: [],
      overlappingControls: [],
      disabledControlsWithoutExplanation: [],
    }),
    ["Review desktop landed on /sign-in instead of /projects/project-123/review"],
  );
});
