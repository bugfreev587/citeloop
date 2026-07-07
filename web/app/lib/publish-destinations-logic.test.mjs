import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadPublishDestinationsModule() {
  const source = await readFile(new URL("./publish-destinations-logic.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

function article(overrides = {}) {
  return {
    id: "article-1",
    kind: "canonical",
    platform: "blog",
    status: "approved",
    content_md: "Draft body",
    scheduled_at: null,
    published_at: null,
    canonical_url: "",
    publish_path: "",
    publish_attempts: 0,
    next_publish_retry_at: null,
    last_publish_error: null,
    seo_meta: { title: "Article title", slug: "article-title" },
    ...overrides,
  };
}

function connection(overrides = {}) {
  return {
    id: "publisher-1",
    project_id: "project-1",
    kind: "github_nextjs",
    label: "Marketing site",
    status: "connected",
    is_default: true,
    enabled: true,
    capabilities: {},
    capability_schema_version: 1,
    credential_configured: true,
    config: { repo: "acme/site", content_dir: "content/blog" },
    last_error: null,
    ...overrides,
  };
}

function distributeItem(overrides = {}) {
  return {
    article: article({
      id: "variant-1",
      kind: "syndication_variant",
      platform: "dev_to",
      status: "approved",
      content_md: "Variant body",
    }),
    compose_url: "https://dev.to/new",
    supports_canonical: true,
    ...overrides,
  };
}

test("buildPublishDestinations maps GitHub Next.js connection state independently from project cadence", async () => {
  const { buildPublishDestinations } = await loadPublishDestinationsModule();

  assert.equal(
    buildPublishDestinations({
      projectId: "project-1",
      connections: [connection()],
      readyDistribute: [],
      waitingSyndication: [],
      projectConfig: { publish_mode: "manual" },
    }).github.stateLabel,
    "Ready",
  );
  assert.equal(
    buildPublishDestinations({
      projectId: "project-1",
      connections: [connection()],
      readyDistribute: [],
      waitingSyndication: [],
      projectConfig: { publish_mode: "manual" },
    }).github.actionLabel,
    "Ready",
  );

  assert.equal(
    buildPublishDestinations({
      projectId: "project-1",
      connections: [],
      readyDistribute: [],
      waitingSyndication: [],
      projectConfig: { publish_mode: "auto" },
    }).github.stateLabel,
    "Not connected",
  );

  assert.equal(
    buildPublishDestinations({
      projectId: "project-1",
      connections: [connection({ enabled: false })],
      readyDistribute: [],
      waitingSyndication: [],
      projectConfig: { publish_mode: "auto" },
    }).github.stateLabel,
    "Disabled",
  );

  assert.equal(
    buildPublishDestinations({
      projectId: "project-1",
      connections: [connection({ status: "error", last_error: "publisher credential unavailable" })],
      readyDistribute: [],
      waitingSyndication: [],
      projectConfig: { publish_mode: "scheduled" },
    }).github.stateLabel,
    "Needs attention",
  );

  assert.equal(
    buildPublishDestinations({
      projectId: "project-1",
      connections: [connection({ status: "revoked", last_error: "Token revoked" })],
      readyDistribute: [],
      waitingSyndication: [],
      projectConfig: { publish_mode: "scheduled" },
    }).github.stateLabel,
    "Needs attention",
  );
});

test("buildPublishDestinations keeps first viewport concise and treats roadmap CMS as unavailable", async () => {
  const { buildPublishDestinations } = await loadPublishDestinationsModule();

  const base = buildPublishDestinations({
    projectId: "project-1",
    connections: [connection()],
    readyDistribute: [],
    waitingSyndication: [],
    projectConfig: { publish_mode: "manual" },
  });

  assert.deepEqual(
    base.firstViewport.map((destination) => destination.id),
    ["github_nextjs", "dev_to", "hashnode", "reddit", "cms_roadmap"],
  );
  assert.deepEqual(
    base.moreManual.map((destination) => destination.id),
    ["medium", "linkedin", "hacker_news"],
  );
  assert.deepEqual(base.roadmap.platforms, ["WordPress", "Webflow", "Shopify", "Custom CMS"]);
  assert.equal(base.roadmap.isPublishAction, false);

  const withLinkedInVariant = buildPublishDestinations({
    projectId: "project-1",
    connections: [connection()],
    readyDistribute: [
      distributeItem({
        article: article({ id: "linkedin-ready", kind: "syndication_variant", platform: "linkedin" }),
        compose_url: "https://www.linkedin.com/article/new/",
      }),
    ],
    waitingSyndication: [],
    projectConfig: { publish_mode: "manual" },
  });

  assert.ok(withLinkedInVariant.firstViewport.some((destination) => destination.id === "linkedin"));
  assert.ok(!withLinkedInVariant.moreManual.some((destination) => destination.id === "linkedin"));
});

test("buildReadyNow shows due approved canonicals and retryable failures without review language", async () => {
  const { buildReadyNow } = await loadPublishDestinationsModule();
  const now = new Date("2026-07-02T12:00:00.000Z");

  const readyNow = buildReadyNow({
    now,
    approvedCanonicals: [
      article({ id: "due", scheduled_at: "2026-07-02T11:59:00.000Z" }),
      article({ id: "unscheduled", scheduled_at: null }),
      article({ id: "future", scheduled_at: "2026-07-05T11:59:00.000Z" }),
    ],
    failedCanonicals: [
      article({
        id: "failed",
        status: "publish_failed",
        publish_attempts: 2,
        last_publish_error: "Build failed",
      }),
    ],
    inflightCanonicals: [article({ id: "verifying", status: "pending_url_verification" })],
    activePublisherConnection: connection(),
    githubState: "ready",
    publishMode: "manual",
  });

  assert.deepEqual(
    readyNow.items.map((item) => [item.articleId, item.actionLabel, item.destinationLabel, item.destinationActionLabel, item.publishStateLabel, "timingActionLabel" in item]),
    [
      ["due", "Publish", "GitHub/Next.js", "Destination", "Manual: publish when ready", false],
      ["unscheduled", "Publish", "GitHub/Next.js", "Destination", "Manual: publish when ready", false],
      ["verifying", "Publishing", "GitHub/Next.js", "Destination", "Verifying live URL", false],
      ["failed", "Retry", "GitHub/Next.js", "Destination", "Failed", false],
    ],
  );
  assert.equal(readyNow.items.some((item) => item.articleId === "future"), false);
  assert.equal(readyNow.items.every((item) => item.secondaryActionLabel === "Preview"), true);
  assert.equal(JSON.stringify(readyNow).includes("Review"), false);
});

test("buildReadyNow disables publish and retry actions when no publisher is active", async () => {
  const { buildReadyNow } = await loadPublishDestinationsModule();

  const readyNow = buildReadyNow({
    now: new Date("2026-07-02T12:00:00.000Z"),
    approvedCanonicals: [article({ id: "due" })],
    failedCanonicals: [article({ id: "failed", status: "publish_failed" })],
    activePublisherConnection: null,
    githubState: "not_connected",
  });

  assert.deepEqual(
    readyNow.items.map((item) => [item.articleId, item.disabledReason]),
    [
      ["due", "Connect GitHub before publishing."],
      ["failed", "Connect GitHub before retrying."],
    ],
  );

  const disabledReady = buildReadyNow({
    now: new Date("2026-07-02T12:00:00.000Z"),
    approvedCanonicals: [article({ id: "due" })],
    failedCanonicals: [article({ id: "failed", status: "publish_failed" })],
    activePublisherConnection: null,
    githubState: "disabled",
  });
  assert.deepEqual(
    disabledReady.items.map((item) => [item.articleId, item.disabledReason]),
    [
      ["due", "Enable GitHub/Next.js publishing before publishing."],
      ["failed", "Enable GitHub/Next.js publishing before retrying."],
    ],
  );

  const attentionReady = buildReadyNow({
    now: new Date("2026-07-02T12:00:00.000Z"),
    approvedCanonicals: [article({ id: "due" })],
    failedCanonicals: [article({ id: "failed", status: "publish_failed" })],
    activePublisherConnection: null,
    githubState: "needs_attention",
  });
  assert.deepEqual(
    attentionReady.items.map((item) => [item.articleId, item.disabledReason]),
    [
      ["due", "Fix GitHub/Next.js before publishing."],
      ["failed", "Fix GitHub/Next.js before retrying."],
    ],
  );
});

test("buildManualSyndicationSummary counts only unlocked variants and keeps waiting rows passive", async () => {
  const { buildManualSyndicationSummary } = await loadPublishDestinationsModule();

  const summary = buildManualSyndicationSummary({
    readyDistribute: [
      distributeItem({ article: article({ id: "dev-1", kind: "syndication_variant", platform: "dev_to" }) }),
      distributeItem({ article: article({ id: "dev-2", kind: "syndication_variant", platform: "dev_to" }) }),
      distributeItem({
        article: article({ id: "reddit-1", kind: "syndication_variant", platform: "reddit" }),
        compose_url: "https://www.reddit.com/submit?type=TEXT",
        supports_canonical: false,
      }),
    ],
    waitingSyndication: [
      article({ id: "waiting-dev", kind: "syndication_variant", platform: "dev_to" }),
      article({ id: "waiting-hn", kind: "syndication_variant", platform: "hacker_news" }),
    ],
  });

  const devTo = summary.platforms.find((platform) => platform.id === "dev_to");
  const reddit = summary.platforms.find((platform) => platform.id === "reddit");
  const hackerNews = summary.platforms.find((platform) => platform.id === "hacker_news");

  assert.equal(devTo.readyCount, 2);
  assert.equal(devTo.waitingCount, 1);
  assert.equal(reddit.actionLabel, "Submit draft");
  assert.deepEqual(devTo.readyRows[0].actions, ["Copy", "Open", "Mark distributed"]);
  assert.deepEqual(devTo.waitingRows[0].actions, []);
  assert.equal(hackerNews.readyCount, 0);
  assert.equal(hackerNews.waitingCount, 1);
});

test("buildPublishingOperationalGroups provides one View all model for non-first-viewport state", async () => {
  const { buildPublishingOperationalGroups } = await loadPublishDestinationsModule();

  const model = buildPublishingOperationalGroups({
    now: new Date("2026-07-02T12:00:00.000Z"),
    approvedCanonicals: [
      article({ id: "ready", scheduled_at: null }),
      article({ id: "scheduled", scheduled_at: "2026-07-05T12:00:00.000Z" }),
    ],
    publishedCanonicals: [article({ id: "published", status: "published", published_at: "2026-07-01T12:00:00.000Z" })],
    failedCanonicals: [article({ id: "failed", status: "publish_failed" })],
    waitingSyndication: [article({ id: "waiting", kind: "syndication_variant", platform: "dev_to" })],
    readyDistribute: [distributeItem({ article: article({ id: "ready-dist", kind: "syndication_variant", platform: "dev_to" }) })],
  });

  assert.deepEqual(
    model.groups.map((group) => [group.key, group.label, group.count]),
    [
      ["ready", "Ready", 1],
      ["scheduled", "Scheduled", 1],
      ["failed", "Failed", 1],
      ["waiting_on_canonical", "Waiting on canonical", 1],
      ["ready_to_distribute", "Ready to distribute", 1],
    ],
  );
  assert.equal(model.published.count, 1);
  assert.equal(model.published.rows[0].publishedUrl, undefined);
  assert.equal(model.published.rows[0].urlMissing, true);
  assert.equal(model.groups.some((group) => group.key === "published"), false);
});

test("buildPublishingOperationalGroups exposes published canonical live URLs without publish_path fallback", async () => {
  const { buildPublishingOperationalGroups } = await loadPublishDestinationsModule();

  const model = buildPublishingOperationalGroups({
    now: new Date("2026-07-02T12:00:00.000Z"),
    approvedCanonicals: [],
    publishedCanonicals: [
      article({
        id: "published-url",
        status: "published",
        published_at: "2026-07-01T12:00:00.000Z",
        canonical_url: "https://example.com/live",
        publish_path: "content/blog/live.mdx",
      }),
      article({
        id: "published-missing-url",
        status: "published",
        published_at: "2026-07-01T13:00:00.000Z",
        canonical_url: "",
        publish_path: "content/blog/missing.mdx",
      }),
    ],
    failedCanonicals: [],
    waitingSyndication: [],
    readyDistribute: [],
  });

  assert.deepEqual(
    model.published.rows.map((row) => [row.articleId, row.publishedUrl, row.urlMissing]),
    [
      ["published-url", "https://example.com/live", false],
      ["published-missing-url", undefined, true],
    ],
  );
});

test("buildPublishHeaderCta follows the PRD state table", async () => {
  const { buildPublishDestinations, buildPublishHeaderCta, buildReadyNow } = await loadPublishDestinationsModule();

  const notConnected = buildPublishDestinations({
    projectId: "project-1",
    connections: [],
    readyDistribute: [],
    waitingSyndication: [],
    projectConfig: { publish_mode: "manual" },
  });
  assert.deepEqual(buildPublishHeaderCta({ projectId: "project-1", github: notConnected.github, readyNowItems: [], scheduledCount: 0 }), {
    label: "Connect GitHub",
    kind: "settings",
    href: "/projects/project-1/settings#publisher",
  });

  const connected = buildPublishDestinations({
    projectId: "project-1",
    connections: [connection()],
    readyDistribute: [],
    waitingSyndication: [],
    projectConfig: { publish_mode: "manual" },
  });
  const readyNow = buildReadyNow({
    now: new Date("2026-07-02T12:00:00.000Z"),
    approvedCanonicals: [article({ id: "ready" })],
    failedCanonicals: [],
    activePublisherConnection: connection(),
  });
  assert.equal(buildPublishHeaderCta({ projectId: "project-1", github: connected.github, readyNowItems: readyNow.items, scheduledCount: 0 }), null);

  assert.deepEqual(buildPublishHeaderCta({ projectId: "project-1", github: connected.github, readyNowItems: [], scheduledCount: 2 }), {
    label: "View schedule",
    kind: "view_all",
    groupKey: "scheduled",
  });
});
