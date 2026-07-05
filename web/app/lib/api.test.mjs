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

test("createApi aborts stalled backend requests before a Vercel function timeout", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (_url, init) => {
    assert.ok(init.signal, "fetch should receive an AbortSignal");
    return new Promise((_resolve, reject) => {
      init.signal.addEventListener("abort", () => {
        reject(init.signal.reason ?? new Error("aborted"));
      });
    });
  };

  try {
    const { createApi } = await loadApiModule();

    await assert.rejects(
      () => createApi({ token: "session-token", timeoutMs: 1 }).listProjects(),
      /CiteLoop API request timed out/,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("createApi retries idempotent GET requests once after a timeout", async () => {
  let calls = 0;
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (_url, init) => {
    calls += 1;
    assert.ok(init.signal, "fetch should receive an AbortSignal");
    if (calls === 1) {
      return new Promise((_resolve, reject) => {
        init.signal.addEventListener("abort", () => {
          reject(init.signal.reason ?? new Error("aborted"));
        });
      });
    }
    return { ok: true, status: 200, json: async () => [{ id: "project-1", name: "unipost.dev", slug: "unipost-dev", config: {} }] };
  };

  try {
    const { createApi } = await loadApiModule();
    const projects = await createApi({ token: "session-token", timeoutMs: 1 }).listProjects();

    assert.equal(calls, 2);
    assert.equal(projects[0].id, "project-1");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("admin destructive deletes use an extended timeout for cascading cleanup", async () => {
  const calls = [];
  const timeouts = [];
  const originalFetch = globalThis.fetch;
  const originalSetTimeout = globalThis.setTimeout;
  const originalClearTimeout = globalThis.clearTimeout;

  globalThis.setTimeout = (handler, delay, ...args) => {
    timeouts.push(delay);
    return originalSetTimeout(handler, 0, ...args);
  };
  globalThis.clearTimeout = (id) => originalClearTimeout(id);
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({
        id: "3ee1f12c-42d0-4b18-be7a-13cacb348778",
        owner_id: "user_123",
        owner_email: "owner@example.test",
        name: "unipost.dev",
        slug: "unipost-dev",
        config: {},
        deleted_projects: 1,
      }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const api = createApi({ token: "session-token" });

    await api.deleteAdminProject("3ee1f12c-42d0-4b18-be7a-13cacb348778");
    await api.deleteAdminUser("user_123");

    assert.deepEqual(
      calls.map((call) => call.url),
      [
        "https://api.example.test/api/admin/projects/3ee1f12c-42d0-4b18-be7a-13cacb348778",
        "https://api.example.test/api/admin/users/user_123",
      ],
    );
    assert.deepEqual(
      timeouts,
      [120_000, 120_000],
      "admin deletes need enough time to wait for project jobs and database cascades instead of the 8s read timeout",
    );
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
  }
});

test("friendlyApiError maps missing project responses to onboarding copy", async () => {
  const { ApiError, friendlyApiError, isProjectMissingError } = await loadApiModule();
  const badProject = new ApiError(400, '{"error":"bad project id"}');
  const missingProject = new ApiError(404, '{"error":"project not found"}');

  assert.equal(isProjectMissingError(badProject), true);
  assert.equal(isProjectMissingError(missingProject), true);
  assert.equal(friendlyApiError(badProject), "Connect your domain to create your first project.");
  assert.equal(friendlyApiError(missingProject), "Connect your domain to create your first project.");
  assert.doesNotMatch(friendlyApiError(badProject), /400|bad project id|\{"error"/);
});

test("project config exposes content plan auto advance and defaults it off", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => ({
    ok: true,
    status: 200,
    json: async () => [
      { id: "manual-project", name: "Manual", slug: "manual", config: {} },
      { id: "auto-project", name: "Auto", slug: "auto", config: { auto_advance_enabled: true } },
    ],
  });

  try {
    const { createApi, defaultProjectConfig } = await loadApiModule();
    assert.equal(defaultProjectConfig().auto_advance_enabled, false);

    const projects = await createApi({ token: "session-token" }).listProjects();
    assert.equal(projects[0].config.auto_advance_enabled, false);
    assert.equal(projects[1].config.auto_advance_enabled, true);
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
      model: "gpt-5.1",
      writer_model: "gpt-5.1",
      qa_model: "gpt-5.5",
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
    assert.equal(status.model, "gpt-5.1");
    assert.equal(status.writer_model, "gpt-5.1");
    assert.equal(status.qa_model, "gpt-5.5");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("updateLLMCredentials sends TokenGate base URL and role models", async () => {
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
        model: "gpt-5.1",
        writer_model: "gpt-5.1",
        qa_model: "gpt-5.5",
      }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi().updateLLMCredentials({
      provider: "tokengate",
      api_key: "tg-test-key",
      base_url: "https://tokengate-production.up.railway.app/v1",
      model: "gpt-5.1",
      writer_model: "gpt-5.1",
      qa_model: "gpt-5.5",
    });

    assert.equal(calls[0].url, "https://api.example.test/api/admin/llm-credentials");
    assert.equal(calls[0].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[0].init.body), {
      provider: "tokengate",
      api_key: "tg-test-key",
      base_url: "https://tokengate-production.up.railway.app/v1",
      model: "gpt-5.1",
      writer_model: "gpt-5.1",
      qa_model: "gpt-5.5",
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GEO credential APIs use admin TokenGate provider endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    if (url.endsWith("/test")) {
      return {
        ok: true,
        status: 200,
        json: async () => ({ ok: true, provider: "tokengate_perplexity", model: "sonar-pro", latency_ms: 42 }),
      };
    }
    if (init.method === "DELETE") {
      return {
        ok: true,
        status: 200,
        json: async () => ({ scope: "perplexity", provider: "tokengate", configured: false, enabled: false }),
      };
    }
    if (init.method === "PUT") {
      return {
        ok: true,
        status: 200,
        json: async () => ({ scope: "perplexity", provider: "tokengate", configured: true, enabled: true, key_tail: "abcd" }),
      };
    }
    return {
      ok: true,
      status: 200,
      json: async () => [
        { scope: "perplexity", provider: "tokengate", configured: true, enabled: true, key_tail: "abcd", model: "sonar-pro" },
        { scope: "openai", provider: "tokengate", configured: false, enabled: false, model: "gpt-5.1" },
      ],
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const api = createApi({ token: "session-token" });

    const statuses = await api.listGEOCredentials();
    await api.updateGEOCredentials("perplexity", {
      provider: "tokengate",
      api_key: "tg-perplexity-key",
      base_url: "https://tokengate-production.up.railway.app/v1",
      model: "sonar-pro",
      enabled: true,
    });
    await api.testGEOCredentials("perplexity");
    await api.deleteGEOCredentials("perplexity");

    assert.equal(statuses[0].scope, "perplexity");
    assert.equal(statuses[0].provider, "tokengate");
    assert.deepEqual(
      calls.map((call) => [call.url, call.init.method ?? "GET"]),
      [
        ["https://api.example.test/api/admin/geo-credentials", "GET"],
        ["https://api.example.test/api/admin/geo-credentials/perplexity", "PUT"],
        ["https://api.example.test/api/admin/geo-credentials/perplexity/test", "POST"],
        ["https://api.example.test/api/admin/geo-credentials/perplexity", "DELETE"],
      ],
    );
    assert.deepEqual(JSON.parse(calls[1].init.body), {
      provider: "tokengate",
      api_key: "tg-perplexity-key",
      base_url: "https://tokengate-production.up.railway.app/v1",
      model: "sonar-pro",
      enabled: true,
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

test("refreshContext calls the fixed-domain context refresh endpoint", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 202,
      json: async () => ({ id: "profile-1", project_id: "project-1", profile: {}, source_urls: [] }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    await createApi().refreshContext("project-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/context/refresh");
    assert.equal(calls[0].init.method, "POST");
    assert.equal(calls[0].init.body, undefined);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("generateTopic normalizes accepted background generation responses", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 202,
      json: async () => ({
        status: "generating",
        topic: { id: "topic-1", project_id: "project-1", title: "Draft me", status: "generating" },
        articles: null,
      }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const result = await createApi().generateTopic("project-1", "topic-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/topics/topic-1/generate");
    assert.equal(calls[0].init.method, "POST");
    assert.equal(result.status, "generating");
    assert.equal(result.topic.id, "topic-1");
    assert.deepEqual(result.articles, []);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("planSEOContentAction creates a topic from an accepted opportunity action", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({
        id: "topic-1",
        project_id: "project-1",
        channel: "blog",
        title: "Draft me",
        status: "backlog",
        source_content_action_id: "action-1",
      }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const topic = await createApi().planSEOContentAction("project-1", "action-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/seo/actions/action-1/plan");
    assert.equal(calls[0].init.method, "POST");
    assert.equal(topic.id, "topic-1");
    assert.equal(topic.source_content_action_id, "action-1");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("list APIs tolerate null responses as empty arrays", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => ({
    ok: true,
    status: 200,
    json: async () => null,
  });

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    assert.deepEqual(await client.listProjects(), []);
    assert.deepEqual(await client.listTopics("project-1"), []);
    assert.deepEqual(await client.listArticles("project-1", "published"), []);
    assert.deepEqual(await client.listRuns("project-1"), []);
    assert.deepEqual(await client.listSEOOpportunities("project-1"), []);
    assert.deepEqual(await client.listAutopilotPlans("project-1"), []);
    assert.deepEqual(await client.listSafeModeEvents("project-1"), []);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("createProject supports URL-first onboarding payloads", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 201,
      json: async () => ({ id: "project-1", name: "unipost.dev", slug: "unipost-dev" }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const project = await createApi().createProject({ site_url: "https://unipost.dev" });

    assert.equal(project.id, "project-1");
    assert.equal(calls[0].url, "https://api.example.test/api/projects");
    assert.deepEqual(JSON.parse(calls[0].init.body), { site_url: "https://unipost.dev" });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("deleteProject hard-deletes through the project endpoint", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({ id: "project-1", name: "unipost.dev", slug: "unipost-dev" }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const project = await createApi().deleteProject("project-1");

    assert.equal(project.id, "project-1");
    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/");
    assert.equal(calls[0].init.method, "DELETE");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("SEO APIs normalize null nested arrays from cold-start projects", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url) => ({
    ok: true,
    status: 200,
    json: async () => {
      if (url.endsWith("/seo/overview")) {
        return {
          property: null,
          integrations: null,
          setup_checklist: [
            { key: "publisher_write", label: "Publishing", status: "in_progress", next_action: "Save token" },
          ],
          capability_mode: "customer_site_pending_verification",
          last_28_days: null,
          technical: null,
          opportunities_by_type: null,
          actions_by_status: null,
          data_source_warnings: null,
          cold_start: true,
        };
      }
      if (url.endsWith("/seo/settings")) {
        return { property: null, integrations: null };
      }
      if (url.endsWith("/seo/briefs/latest")) {
        return { mode: "cold_start", title: "Brief", actions: null, blockers: null, geo_blockers: null, geo_opportunities: null, measurement_updates: null };
      }
      return null;
    },
  });

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    const overview = await client.getSEOOverview("project-1");
    assert.deepEqual(overview.integrations, []);
    assert.deepEqual(overview.opportunities_by_type, []);
    assert.deepEqual(overview.actions_by_status, []);
    assert.deepEqual(overview.data_source_warnings, []);
    assert.equal(overview.capability_mode, "customer_site_pending_verification");
    assert.equal(overview.setup_checklist[0].key, "publisher_write");

    const settings = await client.getSEOSettings("project-1");
    assert.deepEqual(settings.integrations, []);

    const brief = await client.getSEOBrief("project-1");
    assert.deepEqual(brief.actions, []);
    assert.deepEqual(brief.blockers, []);
    assert.deepEqual(brief.geo_blockers, []);
    assert.deepEqual(brief.geo_opportunities, []);
    assert.deepEqual(brief.measurement_updates, []);
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
    await client.fixArticle("project-1", "article-1");
    await client.applyFix("project-1", "article-1", "Remove unsupported claim");
    await client.approve("project-1", "article-1");
    await client.reject("project-1", "article-1");
    await client.distributed("project-1", "article-1");
    await client.retryPublish("project-1", "article-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/articles/article-1");
    assert.equal(calls[0].init.method, undefined);
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/articles/article-1");
    assert.equal(calls[1].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[1].init.body), { content_md: "Body" });
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/articles/article-1/ai-fix");
    assert.equal(calls[2].init.method, "POST");
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/articles/article-1/apply-fix");
    assert.equal(calls[3].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[3].init.body), { instruction: "Remove unsupported claim" });
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/articles/article-1/approve");
    assert.equal(calls[4].init.method, "POST");
    assert.equal(calls[5].url, "https://api.example.test/api/projects/project-1/articles/article-1/reject");
    assert.equal(calls[5].init.method, "POST");
    assert.equal(calls[6].url, "https://api.example.test/api/projects/project-1/articles/article-1/distributed");
    assert.equal(calls[6].init.method, "POST");
    assert.equal(calls[7].url, "https://api.example.test/api/projects/project-1/articles/article-1/retry-publish");
    assert.equal(calls[7].init.method, "POST");
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

test("publisher connection APIs call project scoped endpoints without raw token fields", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.endsWith("/publisher-connections")) {
          return [{ id: "publisher-1", kind: "github_nextjs", enabled: true, capabilities: { create_article: true } }];
        }
        return { id: "publisher-1", kind: "github_nextjs", enabled: true, capabilities: { create_article: true } };
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    const connections = await client.listPublisherConnections("project-1");
    await client.upsertGitHubNextJSPublisherConnection("project-1", {
      repo: "owner/site",
      branch: "staging",
      content_dir: "content/blog",
      base_url: "https://example.com/blog",
      publish_mode: "publish",
    });
    await client.upsertPublisherCredential("project-1", "publisher-1", {
      kind: "github_token",
      value: "ghp_customer_token",
    });
    await client.testPublisherConnection("project-1", "publisher-1");
    await client.setPublisherConnectionEnabled("project-1", "publisher-1", false);
    await client.revokePublisherCredential("project-1", "publisher-1");
    await client.deletePublisherConnection("project-1", "publisher-1");

    assert.equal(connections[0].capabilities.create_article, true);
    assert.equal(connections[0].enabled, true);
    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/publisher-connections");
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/publisher-connections/github-nextjs");
    assert.equal(calls[1].init.method, "PUT");
    const body = JSON.parse(calls[1].init.body);
    assert.equal(body.repo, "owner/site");
    assert.equal(Object.hasOwn(body, "credential_ref"), false);
    assert.equal(Object.hasOwn(body, "token"), false);
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/publisher-connections/publisher-1/credential");
    assert.equal(calls[2].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[2].init.body), { kind: "github_token", value: "ghp_customer_token" });
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/publisher-connections/publisher-1/test");
    assert.equal(calls[3].init.method, "POST");
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/publisher-connections/publisher-1/enabled");
    assert.equal(calls[4].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[4].init.body), { enabled: false });
    assert.equal(calls[5].url, "https://api.example.test/api/projects/project-1/publisher-connections/publisher-1/credential");
    assert.equal(calls[5].init.method, "DELETE");
    assert.equal(calls[6].url, "https://api.example.test/api/projects/project-1/publisher-connections/publisher-1");
    assert.equal(calls[6].init.method, "DELETE");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("SEO APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.endsWith("/seo/overview")) {
          return {
            integrations: [],
            last_28_days: {},
            technical: {},
            opportunities_by_type: [],
            actions_by_status: [],
            cold_start: true,
            handoff_ready_for_autopilot: false,
            setup_checklist: [
              { key: "publisher_write", label: "Publishing", status: "in_progress", next_action: "Save token" },
            ],
            capability_mode: "customer_site_pending_verification",
          };
        }
        if (url.endsWith("/seo/settings")) {
          return { property: null, integrations: [] };
        }
        if (url.endsWith("/seo/briefs/latest")) {
          return { mode: "cold_start", title: "Brief", actions: [], blockers: [], measurement_updates: [] };
        }
        if (url.includes("/seo/opportunities") && !url.endsWith("/actions")) {
          return [{ id: "opp-1", type: "indexing_anomaly", status: "open" }];
        }
        if (url.includes("/seo/actions")) {
          return [{ id: "action-1", status: "ready_for_review" }];
        }
        if (url.endsWith("/seo/autopilot/objectives")) {
          return [{ id: "objective-1", name: "Grow clicks", status: "active" }];
        }
        if (url.endsWith("/seo/autopilot/policy")) {
          return { id: "policy-1", autopilot_level: 0, weekly_action_limit: 5 };
        }
        if (url.endsWith("/seo/autopilot/plans/generate")) {
          return { plan: { id: "plan-1", actions: [] }, run: { id: "run-1" } };
        }
        if (url.endsWith("/seo/autopilot/plans")) {
          return [{ id: "plan-1", status: "ready_for_review" }];
        }
        if (url.endsWith("/seo/autopilot/safe-mode")) {
          return [{ id: "safe-1", reason: "manual" }];
        }
        return { status: "ok" };
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    const overview = await client.getSEOOverview("project-1");
    await client.getSEOSettings("project-1");
    await client.updateSEOSettings("project-1", {
      site_url: "https://dev.unipost.dev",
      gsc_site_url: "sc-domain:unipost.dev",
      gsc_credential_ref: "GOOGLE_SERVICE_ACCOUNT_JSON",
    });
    await client.syncSEO("project-1", "https://dev.unipost.dev");
    await client.getSEOBrief("project-1");
    await client.listSEOOpportunities("project-1", { status: "open", limit: 10 });
    await client.createSEOContentAction("project-1", "opp-1", { action_type: "technical SEO fix task" });
    await client.listSEOContentActions("project-1", { limit: 10 });
    await client.listSEOObjectives("project-1");
    await client.createSEOObjective("project-1", { name: "Grow clicks" });
    await client.getSEOPolicy("project-1");
    await client.updateSEOPolicy("project-1", { autopilot_level: 1, weekly_action_limit: 3 });
    await client.generateAutopilotPlan("project-1");
    await client.listAutopilotPlans("project-1");
    await client.listSafeModeEvents("project-1");
    await client.enterSafeMode("project-1", { reason: "manual" });
    await client.exitSafeMode("project-1", "safe-1", { exited_by: "human", exit_reason: "reviewed" });

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/seo/overview");
    assert.equal(overview.capability_mode, "customer_site_pending_verification");
    assert.equal(overview.setup_checklist[0].key, "publisher_write");
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/seo/settings");
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/seo/settings");
    assert.equal(calls[2].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[2].init.body), {
      site_url: "https://dev.unipost.dev",
      gsc_site_url: "sc-domain:unipost.dev",
      gsc_credential_ref: "GOOGLE_SERVICE_ACCOUNT_JSON",
    });
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/seo/sync");
    assert.equal(calls[3].init.method, "POST");
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/seo/briefs/latest");
    assert.equal(calls[5].url, "https://api.example.test/api/projects/project-1/seo/opportunities?status=open&limit=10");
    assert.equal(calls[6].url, "https://api.example.test/api/projects/project-1/seo/opportunities/opp-1/actions");
    assert.equal(calls[6].init.method, "POST");
    assert.equal(calls[7].url, "https://api.example.test/api/projects/project-1/seo/actions?limit=10");
    assert.equal(calls[8].url, "https://api.example.test/api/projects/project-1/seo/autopilot/objectives");
    assert.equal(calls[9].url, "https://api.example.test/api/projects/project-1/seo/autopilot/objectives");
    assert.equal(calls[9].init.method, "POST");
    assert.equal(calls[10].url, "https://api.example.test/api/projects/project-1/seo/autopilot/policy");
    assert.equal(calls[11].url, "https://api.example.test/api/projects/project-1/seo/autopilot/policy");
    assert.equal(calls[11].init.method, "PUT");
    assert.equal(calls[12].url, "https://api.example.test/api/projects/project-1/seo/autopilot/plans/generate");
    assert.equal(calls[13].url, "https://api.example.test/api/projects/project-1/seo/autopilot/plans");
    assert.equal(calls[14].url, "https://api.example.test/api/projects/project-1/seo/autopilot/safe-mode");
    assert.equal(calls[15].url, "https://api.example.test/api/projects/project-1/seo/autopilot/safe-mode");
    assert.equal(calls[15].init.method, "POST");
    assert.equal(calls[16].url, "https://api.example.test/api/projects/project-1/seo/autopilot/safe-mode/safe-1/exit");
    assert.equal(calls[16].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[16].init.body), { exited_by: "human", exit_reason: "reviewed" });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("results action normalization does not invent verification snapshots", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => ({
    ok: true,
    status: 200,
    json: async () => [
      {
        id: "action-1",
        opportunity_id: "opp-1",
        action_type: "Create content",
        status: "ready_for_review",
      },
    ],
  });

  try {
    const { createApi } = await loadApiModule();
    const actions = await createApi().listResultsActions("project-1");

    assert.equal(actions[0].verification_snapshot, null);
    assert.equal(actions[0].verified_at, undefined);
    assert.equal(actions[0].published_at, undefined);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GSC OAuth APIs call project scoped endpoints without exposing tokens", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.endsWith("/seo/gsc/connection")) {
          return {
            configured: true,
            status: "property_selection_required",
            selected_property: null,
            recommended_property: "sc-domain:unipost.dev",
            properties: [
              { site_url: "sc-domain:unipost.dev", permission_level: "siteOwner", recommended: true },
            ],
          };
        }
        if (url.endsWith("/seo/gsc/oauth/start")) {
          return { authorization_url: "https://accounts.google.com/o/oauth2/v2/auth?state=state-1" };
        }
        if (url.endsWith("/seo/gsc/oauth/complete")) {
          return {
            configured: true,
            status: "property_selection_required",
            selected_property: null,
            properties: [
              { site_url: "sc-domain:unipost.dev", permission_level: "siteOwner", recommended: true },
            ],
          };
        }
        if (url.endsWith("/seo/gsc/property")) {
          return { configured: true, status: "connected", selected_property: "sc-domain:unipost.dev", properties: [] };
        }
        if (url.endsWith("/seo/gsc/revoke")) {
          return { configured: true, status: "revoked", selected_property: null, properties: [] };
        }
        return {};
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    const connection = await client.getGSCConnection("project-1");
    const start = await client.startGSCOAuth("project-1");
    const completed = await client.completeGSCOAuth("project-1", { code: "code-1", state: "state-1" });
    const selected = await client.selectGSCProperty("project-1", { site_url: "sc-domain:unipost.dev" });
    const revoked = await client.revokeGSCConnection("project-1");

    assert.equal(connection.recommended_property, "sc-domain:unipost.dev");
    assert.equal(Object.hasOwn(connection, "refresh_token"), false);
    assert.match(start.authorization_url, /^https:\/\/accounts\.google\.com/);
    assert.equal(completed.properties[0].site_url, "sc-domain:unipost.dev");
    assert.equal(selected.status, "connected");
    assert.equal(revoked.status, "revoked");
    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/seo/gsc/connection");
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/seo/gsc/oauth/start");
    assert.equal(calls[1].init.method, "POST");
    assert.equal(calls[1].init.body, undefined);
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/seo/gsc/oauth/complete");
    assert.equal(calls[2].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[2].init.body), { code: "code-1", state: "state-1" });
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/seo/gsc/property");
    assert.equal(calls[3].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[3].init.body), { site_url: "sc-domain:unipost.dev" });
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/seo/gsc/revoke");
    assert.equal(calls[4].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GEO crawler APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.endsWith("/geo/crawler-audit/latest")) {
          return { snapshots: [] };
        }
        return { checked_urls: 1, created_blockers: 0, snapshots: [] };
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    const run = await client.runGEOCrawlerAudit("project-1", { target_user_agents: ["OAI-SearchBot"] });
    const latest = await client.getLatestGEOCrawlerAudit("project-1");

    assert.equal(run.checked_urls, 1);
    assert.deepEqual(latest.snapshots, []);
    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/geo/crawler-audit");
    assert.equal(calls[0].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[0].init.body), { target_user_agents: ["OAI-SearchBot"] });
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/geo/crawler-audit/latest");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GEO PR2 APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.endsWith("/geo/overview")) {
          return { score: null, prompt_sets: [], prompts: [], competitors: [], external_surfaces: [], observations: [] };
        }
        if (url.endsWith("/geo/prompt-sets")) {
          return { prompt_sets: [], prompts: [], competitors: [] };
        }
        if (url.endsWith("/geo/observations")) {
          return [];
        }
        if (url.endsWith("/geo/external-surfaces")) {
          return [];
        }
        return { id: "geo-row-1", prompts: [], observations: [], score: null };
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    await client.getGEOOverview("project-1");
    await client.generateGEOPromptSet("project-1", { locale: "en-US" });
    await client.listGEOPromptSets("project-1");
    await client.updateGEOPromptSet("project-1", "set-1", { status: "active" });
    await client.updateGEOPrompt("project-1", "prompt-1", { status: "paused" });
    await client.updateGEOCompetitor("project-1", "competitor-1", { status: "paused" });
    await client.observeGEOManualFixtures("project-1", {
      engine: "Perplexity",
      observations: [{ prompt_id: "prompt-1", brand_mentioned: true, cited_urls: ["https://unipost.dev/blog"] }],
    });
    await client.listGEOObservations("project-1", { limit: 10 });
    await client.listGEOExternalSurfaces("project-1");
    await client.createGEOExternalSurface("project-1", { url: "https://dev.to/unipost/guide", owner_type: "project" });

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/geo/overview");
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/geo/prompt-sets/generate");
    assert.equal(calls[1].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[1].init.body), { locale: "en-US" });
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/geo/prompt-sets");
    assert.equal(calls[3].url, "https://api.example.test/api/projects/project-1/geo/prompt-sets/set-1");
    assert.equal(calls[3].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[3].init.body), { status: "active" });
    assert.equal(calls[4].url, "https://api.example.test/api/projects/project-1/geo/prompts/prompt-1");
    assert.equal(calls[4].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[4].init.body), { status: "paused" });
    assert.equal(calls[5].url, "https://api.example.test/api/projects/project-1/geo/competitors/competitor-1");
    assert.equal(calls[5].init.method, "PUT");
    assert.deepEqual(JSON.parse(calls[5].init.body), { status: "paused" });
    assert.equal(calls[6].url, "https://api.example.test/api/projects/project-1/geo/runs/observe");
    assert.equal(calls[6].init.method, "POST");
    assert.equal(calls[7].url, "https://api.example.test/api/projects/project-1/geo/observations?limit=10");
    assert.equal(calls[8].url, "https://api.example.test/api/projects/project-1/geo/external-surfaces");
    assert.equal(calls[9].url, "https://api.example.test/api/projects/project-1/geo/external-surfaces");
    assert.equal(calls[9].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GEO PR3 APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => {
        if (url.endsWith("/geo/asset-briefs")) {
          return [];
        }
        return { opportunities: [], asset_briefs: [], brief: { id: "brief-1" }, topic: { id: "topic-1" } };
      },
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    await client.analyzeGEOOpportunities("project-1", { limit: 25 });
    await client.listGEOAssetBriefs("project-1");
    await client.acceptGEOAssetBrief("project-1", "brief-1");

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/geo/opportunities/analyze");
    assert.equal(calls[0].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[0].init.body), { limit: 25 });
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/geo/asset-briefs");
    assert.equal(calls[2].url, "https://api.example.test/api/projects/project-1/geo/asset-briefs/brief-1/accept");
    assert.equal(calls[2].init.method, "POST");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GEO PR4 provider and surface monitor APIs call project scoped endpoints", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => ({ run: { id: "run-1" }, observations: [], surfaces: [], checked: 0 }),
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    await client.observeGEOProvider("project-1", { engine: "Perplexity", max_prompts: 5 });
    await client.monitorGEOExternalSurfaces("project-1", { limit: 10 });

    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/geo/runs/observe-provider");
    assert.equal(calls[0].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[0].init.body), { engine: "Perplexity", max_prompts: 5 });
    assert.equal(calls[1].url, "https://api.example.test/api/projects/project-1/geo/external-surfaces/monitor");
    assert.equal(calls[1].init.method, "POST");
    assert.deepEqual(JSON.parse(calls[1].init.body), { limit: 10 });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("GEO runs API calls project scoped endpoint", async () => {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      json: async () => [{ id: "geo-run-1", agent: "geo_observer", status: "degraded" }],
    };
  };

  try {
    const { createApi } = await loadApiModule();
    const client = createApi();

    const runs = await client.listGEORuns("project-1", { agent: "geo_observer", status: "degraded", limit: 10 });

    assert.equal(runs[0].id, "geo-run-1");
    assert.equal(calls[0].url, "https://api.example.test/api/projects/project-1/geo/runs?agent=geo_observer&status=degraded&limit=10");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
