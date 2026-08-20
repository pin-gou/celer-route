/**
 * validate-config-schema.ts
 *
 * TDD green phase: validates fixture config samples against config.schema.json
 * using ajv. The governance fixture includes fields that were added to the
 * schema (disable_auto_tool_inject, routing_chain_max_depth) so validation
 * should now pass — proving the schema is in sync with the Go config struct.
 *
 * Expected: governance fixture PASSES, mocker fixture PASSES.
 */

import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import Ajv2019 from "ajv/dist/2019";
import addFormats from "ajv-formats";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Paths relative to project root (scripts/ is one level deep)
const schemaPath = resolve(__dirname, "..", "transports", "config.schema.json");

function loadJSON(absolutePath: unknown): unknown {
  const raw = readFileSync(absolutePath as string, "utf-8");
  return JSON.parse(raw);
}

// ── Fixture: governance plugin config ──────────────────────────────────
// Includes fields that exist in the Go struct but are NOT yet in the schema:
//   disable_auto_tool_inject, routing_chain_max_depth
// The schema has additionalProperties: false on the governance plugin config,
// so these extra fields trigger a validation error.
const governanceFixture = {
  plugins: [
    {
      name: "governance",
      enabled: true,
      config: {
        is_vk_mandatory: true,
        required_headers: ["x-org-id"],
        // These two fields are in the Go struct but NOT in the schema's governance plugin config section:
        disable_auto_tool_inject: false,
        routing_chain_max_depth: 10,
      },
    },
  ],
};

// ── Fixture: mocker plugin config ──────────────────────────────────────
// A simple mocker config that should pass schema validation (mocker is not
// specially constrained in the schema, so it passes through additionalProperties).
const mockerFixture = {
  plugins: [
    {
      name: "bifrost-mocker",
      enabled: true,
      config: {
        default_behavior: "passthrough",
        routes: [
          {
            path_pattern: "/api/chat/completions",
            method: "POST",
            response_type: "success",
          },
        ],
      },
    },
  ],
};

function main(): void {
  const schema = loadJSON(schemaPath);
  const ajv = new Ajv2019({ strict: false });
  addFormats(ajv);

  const validate = ajv.compile(schema);

  // ── Validate governance fixture ────────────────────────────────────
  const govValid = validate(governanceFixture);
  const govErrors = validate.errors;

  console.log("=== Governance fixture ===");
  if (govValid) {
    console.log("PASS: governance fixture validated successfully");
  } else {
    console.log("FAIL: governance fixture validation failed (expected in red phase)");
    for (const err of govErrors ?? []) {
      console.log(`  - ${err.instancePath} ${err.message} (${JSON.stringify(err.params)})`);
    }
  }

  // ── Validate mocker fixture ────────────────────────────────────────
  const mockerValid = validate(mockerFixture);
  const mockerErrors = validate.errors;

  console.log("\n=== Mocker fixture ===");
  if (mockerValid) {
    console.log("PASS: mocker fixture validated successfully");
  } else {
    console.log("FAIL: mocker fixture validation failed");
    for (const err of mockerErrors ?? []) {
      console.log(`  - ${err.instancePath} ${err.message} (${JSON.stringify(err.params)})`);
    }
  }

  // ── Summary: green phase ───────────────────────────────────────────
  // Expected: both pass (schema has been updated with the missing fields).
  if (govValid && mockerValid) {
    console.log("\n✓ Green phase: both fixtures PASS");
    console.log("  The governance plugin config section now includes disable_auto_tool_inject");
    console.log("  and routing_chain_max_depth. The schema is in sync with the Go config struct.");
    process.exit(0);
  } else if (!govValid) {
    console.log("\n⚠ Green phase violation: governance fixture failed");
    console.log("  The schema is still missing disable_auto_tool_inject and/or routing_chain_max_depth.");
    process.exit(1);
  } else {
    console.log("\n⚠ Green phase violation: mocker fixture also failed");
    console.log("  The mocker fixture should pass generic plugin validation.");
    process.exit(1);
  }
}

main();