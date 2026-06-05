import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

const normalizeStubSource = `
export const normalizeArticle = (value) => value;
export const normalizeInventoryItem = (value) => value;
export const normalizeProfile = (value) => value;
export const normalizeRun = (value) => value;
export const normalizeTopic = (value) => value;
`;

async function loadApiModule() {
  process.env.NEXT_PUBLIC_API_URL = "https://api.example.test";

  const normalizeUrl = `data:text/javascript;base64,${Buffer.from(normalizeStubSource).toString("base64")}`;
  const source = await readFile(new URL("./api.ts", import.meta.url), "utf8");
  const withStubImport = source.replace('} from "./normalize";', `} from ${JSON.stringify(normalizeUrl)};`);
  const transpiled = ts.transpileModule(withStubImport, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("createApi attaches a provided Clerk token as Authorization", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return { ok: true, status: 200, json: async () => [] };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi({ token: "session-token" }).listProjects();

    assert.equal(calls[0].url, "https://api.example.test/api/projects");
    assert.equal(calls[0].init.headers.get("Authorization"), "Bearer session-token");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("createApi resolves Clerk tokens from getToken", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return { ok: true, status: 200, json: async () => ({ provider: "openai", configured: true }) };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi({ getToken: async () => "async-session-token" }).getLLMCredentials();

    assert.equal(calls[0].url, "https://api.example.test/api/admin/llm-credentials");
    assert.equal(calls[0].init.headers.get("Authorization"), "Bearer async-session-token");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("createApi normalizes TokenGate LLM credential status", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => ({
    ok: true,
    status: 200,
    json: async () => ({
      provider: "tokengate",
      configured: true,
      key_tail: "abcd",
      base_url: "https://tokengate-production.up.railway.app/v1",
      updated_at: "2026-06-05T12:00:00Z",
    }),
  });

  try {
    const { createApi } = await loadApiModule();
    const status = await createApi().getLLMCredentials();

    assert.equal(status.provider, "tokengate");
    assert.equal(status.configured, true);
    assert.equal(status.key_tail, "abcd");
    assert.equal(status.base_url, "https://tokengate-production.up.railway.app/v1");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("updateLLMCredentials sends base URL with TokenGate provider", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({
        provider: "tokengate",
        configured: true,
        base_url: "https://tokengate-production.up.railway.app/v1",
      }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi().updateLLMCredentials({
      provider: "tokengate",
      api_key: "tg-test-key",
      base_url: "https://tokengate-production.up.railway.app/v1",
    });

    assert.equal(calls[0].url, "https://api.example.test/api/admin/llm-credentials");
    assert.equal(calls[0].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[0].init.body), {
      provider: "tokengate",
      api_key: "tg-test-key",
      base_url: "https://tokengate-production.up.railway.app/v1",
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("listRuns calls the project runs endpoint", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => [{ id: "run-1", project_id: "project-1", agent: "writer", status: "ok" }],
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const runs = await createApi().listRuns("project-1", { agent: "writer", status: "ok", limit: 25 });

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/runs?agent=writer&status=ok&limit=25");
    assert.equal(runs[0].id, "run-1");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("topic mutation APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({ id: "topic-1", project_id: "project-1", title: "Updated topic" }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    await client.updateTopic("project-1", "topic-1", { title: "Updated topic", priority: 3 });
    await client.scheduleTopic("project-1", "topic-1", "2026-06-10T09:00:00.000Z");
    await client.archiveTopic("project-1", "topic-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/topics/topic-1");
    assert.equal(calls[0].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[0].init.body), { title: "Updated topic", priority: 3 });
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/topics/topic-1/schedule");
    assert.equal(calls[1].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[1].init.body), { scheduled_at: "2026-06-10T09:00:00.000Z" });
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/topics/topic-1/archive");
    assert.equal(calls[2].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("updateInventory sends editable evidence snippets", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({ id: "item-1", evidence_snippets: ["Verified claim"] }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi().updateInventory("project-1", "item-1", {
      title: "Inventory item",
      target_keyword: "keyword",
      topics: ["topic"],
      summary: "Summary",
      evidence_snippets: ["Verified claim"],
    });

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/inventory/item-1");
    assert.equal(calls[0].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[0].init.body), {
      title: "Inventory item",
      target_keyword: "keyword",
      topics: ["topic"],
      summary: "Summary",
      evidence_snippets: ["Verified claim"],
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("article mutation APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({ id: "article-1", project_id: "project-1", content_md: "Body" }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    await client.getArticle("project-1", "article-1");
    await client.edit("project-1", "article-1", { content_md: "Body" });
    await client.approve("project-1", "article-1");
    await client.reject("project-1", "article-1");
    await client.distributed("project-1", "article-1");
    await client.retryPublish("project-1", "article-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/articles/article-1");
    assert.equal(calls[0].init.method, undefined);
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/articles/article-1");
    assert.equal(calls[1].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[1].init.body), { content_md: "Body" });
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/articles/article-1/approve");
    assert.equal(calls[2].init.method, "POST");
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/articles/article-1/reject");
    assert.equal(calls[3].init.method, "POST");
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/articles/article-1/distributed");
    assert.equal(calls[4].init.method, "POST");
    assert.equal(calls[5].url, "https://api.example.test/api/projects/project-1/articles/article-1/retry-publish");
    assert.equal(calls[5].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("notification APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.includes("/channels")) {
          return [{ id: "channel-1", kind: "slack_webhook", config: { redacted_url: "https://hooks.slack.com/***" } }];
        }
        if (url.includes("/subscriptions")) {
          return { id: "sub-1", event_type: "publish.failed", channel_id: "channel-1", enabled: true };
        }
        if (url.includes("/deliveries")) {
          return [{ id: "delivery-1", status: "dead", event_type: "publish.failed" }];
        }
        return [];
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    await client.listNotificationChannels("project-1");
    await client.createNotificationChannel("project-1", {
      kind: "slack_webhook",
      label: "Ops",
      url: "https://hooks.slack.com/services/T/B/token",
    });
    await client.testNotificationChannel("project-1", "channel-1");
    await client.upsertNotificationSubscription("project-1", {
      event_type: "publish.failed",
      channel_id: "channel-1",
      enabled: true,
    });
    await client.listNotificationDeliveries("project-1", { status: "dead", limit: 20 });
    await client.retryNotificationDelivery("project-1", "delivery-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/notifications/channels");
    assert.equal(calls[1].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[1].init.body), {
      kind: "slack_webhook",
      label: "Ops",
      url: "https://hooks.slack.com/services/T/B/token",
    });
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/notifications/channels/channel-1/test");
    assert.equal(calls[2].init.method, "POST");
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/notifications/subscriptions");
    assert.equal(calls[3].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[3].init.body), {
      event_type: "publish.failed",
      channel_id: "channel-1",
      enabled: true,
    });
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/notifications/deliveries?status=dead&limit=20");
    assert.equal(calls[5].url, "https://api.example.test/api/projects/project-1/notifications/deliveries/delivery-1/retry");
    assert.equal(calls[5].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("publishing reconcile API calls project scoped endpoint", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return { ok: true, status: 200, json: async () => ({ status: "reconcile complete" }) };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi().reconcilePublishing("project-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/publishing/reconcile");
    assert.equal(calls[0].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
