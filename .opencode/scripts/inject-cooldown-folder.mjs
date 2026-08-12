#!/usr/bin/env node
// Surgically appends a top-level folder to provider-harness.json. Avoids full
// file reformatting (which breaks diffs across the existing ~50k lines).
// Idempotent: aborts if the folder name already exists.

import { readFileSync, writeFileSync } from "node:fs";

const PATH = "tests/e2e/api/collections/provider-harness.json";
const FOLDER_NAME = "46. Provider-Cooldown (Plugin Wire-Visible Smoke)";

const folder = {
  name: FOLDER_NAME,
  description:
    "Smoke tests that the provider-cooldown plugin does not regress the wire " +
    "path. The plugin operates at the key-pool level (PostLLMHook + " +
    "KeyPoolFilter), so the only directly observable side-effects in normal " +
    "traffic are (a) zero impact on the happy path and (b) the filter keeps " +
    "working after the plugin is reloaded via the API. These cases pin both. " +
    "Filter correctness under real quota errors is covered by Go-level tests " +
    "in plugins/providercooldown/cooldown_test.go (which exercise the state " +
    "machine directly).",
  item: [
    {
      name: "provider-cooldown openai chat smoke - plugin does not break wire",
      event: [
        {
          listen: "test",
          script: {
            type: "text/javascript",
            exec: [
              "if ([401, 403, 429, 500, 502, 503, 504].indexOf(pm.response.code) !== -1) { return; }",
              "pm.test('provider-cooldown openai chat smoke - plugin does not break wire', function () {",
              "  pm.expect(pm.response.code, 'failed: ' + pm.response.text()).to.be.below(400);",
              "  var j = pm.response.json();",
              "  pm.expect(j.choices && j.choices[0] && j.choices[0].message, 'expected a chat completion response').to.be.ok;",
              "});",
            ],
          },
        },
      ],
      request: {
        method: "POST",
        header: [
          { key: "Content-Type", value: "application/json" },
          { key: "Authorization", value: "Bearer {{openaiKey}}" },
        ],
        body: {
          mode: "raw",
          raw: JSON.stringify(
            {
              model: "openai/gpt-4o-mini",
              messages: [{ role: "user", content: "Reply with one word." }],
              max_tokens: 16,
            },
            null,
            2
          ),
        },
        url: {
          raw: "{{baseUrl}}/v1/chat/completions",
          host: ["{{baseUrl}}"],
          path: ["v1", "chat", "completions"],
        },
      },
    },
    {
      name: "provider-cooldown openai streaming smoke - per-chunk PostLLMHook does not crash",
      event: [
        {
          listen: "test",
          script: {
            type: "text/javascript",
            exec: [
              "if ([401, 403, 429, 500, 502, 503, 504].indexOf(pm.response.code) !== -1) { return; }",
              "pm.test('provider-cooldown openai streaming smoke - SSE stream completed', function () {",
              "  var ct = pm.response.headers.get('content-type') || '';",
              "  pm.expect(ct, 'expected SSE, got ' + ct).to.include('event-stream');",
              "  pm.expect(pm.response.text(), 'stream must end with [DONE]').to.include('[DONE]');",
              "});",
            ],
          },
        },
      ],
      request: {
        method: "POST",
        header: [
          { key: "Content-Type", value: "application/json" },
          { key: "Authorization", value: "Bearer {{openaiKey}}" },
          { key: "Accept", value: "text/event-stream" },
        ],
        body: {
          mode: "raw",
          raw: JSON.stringify(
            {
              model: "openai/gpt-4o-mini",
              messages: [{ role: "user", content: "Reply with one word." }],
              max_tokens: 16,
              stream: true,
            },
            null,
            2
          ),
        },
        url: {
          raw: "{{baseUrl}}/v1/chat/completions",
          host: ["{{baseUrl}}"],
          path: ["v1", "chat", "completions"],
        },
      },
    },
    {
      name: "provider-cooldown openai chat smoke after plugin update - filter rewired",
      event: [
        {
          listen: "test",
          script: {
            type: "text/javascript",
            exec: [
              "if ([401, 403, 404, 429, 500, 502, 503, 504].indexOf(pm.response.code) !== -1) { return; }",
              "pm.test('plugin update succeeded (provider-cooldown re-enabled)', function () {",
              "  pm.expect(pm.response.code, 'plugin update failed: ' + pm.response.text()).to.be.below(400);",
              "  var j = pm.response.json();",
              "  pm.expect(j.name, 'plugin name should be provider-cooldown').to.eql('provider-cooldown');",
              "  pm.expect(j.enabled, 'plugin should be enabled after update').to.be.true;",
              "});",
            ],
          },
        },
      ],
      request: {
        method: "PUT",
        header: [{ key: "Content-Type", value: "application/json" }],
        body: {
          mode: "raw",
          raw: JSON.stringify(
            {
              name: "provider-cooldown",
              enabled: true,
              config: {
                default_ttl_seconds: 600,
                ttl_overrides: { openai: 60 },
              },
            },
            null,
            2
          ),
        },
        url: {
          raw: "{{baseUrl}}/api/plugins/provider-cooldown",
          host: ["{{baseUrl}}"],
          path: ["api", "plugins", "provider-cooldown"],
        },
      },
    },
    {
      name: "provider-cooldown openai chat after reload - cooldown filter still wired",
      event: [
        {
          listen: "test",
          script: {
            type: "text/javascript",
            exec: [
              "if ([401, 403, 429, 500, 502, 503, 504].indexOf(pm.response.code) !== -1) { return; }",
              "pm.test('provider-cooldown openai chat after reload - wire intact (BUG-1 regression)', function () {",
              "  pm.expect(pm.response.code, 'failed: ' + pm.response.text()).to.be.below(400);",
              "  var j = pm.response.json();",
              "  pm.expect(j.choices && j.choices[0] && j.choices[0].message, 'expected a chat completion response').to.be.ok;",
              "});",
            ],
          },
        },
      ],
      request: {
        method: "POST",
        header: [
          { key: "Content-Type", value: "application/json" },
          { key: "Authorization", value: "Bearer {{openaiKey}}" },
        ],
        body: {
          mode: "raw",
          raw: JSON.stringify(
            {
              model: "openai/gpt-4o-mini",
              messages: [{ role: "user", content: "Reply with one word." }],
              max_tokens: 16,
            },
            null,
            2
          ),
        },
        url: {
          raw: "{{baseUrl}}/v1/chat/completions",
          host: ["{{baseUrl}}"],
          path: ["v1", "chat", "completions"],
        },
      },
    },
  ],
};

const raw = readFileSync(PATH, "utf8");
const parsed = JSON.parse(raw);

if (!parsed.item || !Array.isArray(parsed.item)) {
  throw new Error("expected top-level item array");
}

const existing = parsed.item.find((it) => it && it.name === FOLDER_NAME);
if (existing) {
  console.error(`[inject-cooldown-folder] folder ${JSON.stringify(FOLDER_NAME)} already exists — aborting (idempotence)`);
  process.exit(0);
}

const beforeCount = parsed.item.length;

// Indent every line of the JSON.stringify by 4 spaces (matches existing top-level folder indent).
const indented = JSON.stringify(folder, null, 2)
  .split("\n")
  .map((l) => "    " + l)
  .join("\n");

const tail = "\n  ]\n}";
if (!raw.endsWith(tail)) {
  throw new Error("unexpected file tail (expected '\\n  ]\\n}')");
}

const out = raw.slice(0, -tail.length) + ",\n" + indented + tail;
const afterParsed = JSON.parse(out);
if (afterParsed.item.length !== beforeCount + 1) {
  throw new Error(`item count expected ${beforeCount + 1}, got ${afterParsed.item.length}`);
}

writeFileSync(PATH, out);
console.log(`[inject-cooldown-folder] inserted folder ${JSON.stringify(FOLDER_NAME)} with ${folder.item.length} cases (items: ${beforeCount} -> ${afterParsed.item.length})`);