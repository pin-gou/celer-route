import { describe, expect, it } from "vitest";

import { RoutingRule } from "@/lib/types/routingRules";
import {
	generateRoutingTestCommandCurl,
	generateRoutingTestCommandGo,
	generateRoutingTestCommandNode,
	generateRoutingTestCommandPython,
} from "@/lib/utils/routingRules";

const rule = {
	id: "r1",
	name: "rule",
	description: "",
	cel_expression: "",
	scope: "global",
	enabled: true,
	priority: 0,
	chain_rule: false,
	query: {
		combinator: "and",
		rules: [{ field: "model", operator: "=", value: "pg-master" }],
	},
	fallbacks: [],
	targets: [],
	created_at: "",
	updated_at: "",
} as RoutingRule;

const BASE = { baseUrl: "http://localhost:3028" } as const;

describe("routing test command snippets", () => {
	describe("when auth is not enforced", () => {
		const opts = { ...BASE, enforceAuth: false } as const;

		it("curl omits the Authorization header", () => {
			const out = generateRoutingTestCommandCurl(rule, opts);
			expect(out).not.toContain("Authorization");
			expect(out).not.toContain("PG_API_KEY");
			expect(out).not.toContain("CELER_ROUTE_API_KEY");
		});

		it("python omits the Authorization header and the os import", () => {
			const out = generateRoutingTestCommandPython(rule, opts);
			expect(out).not.toContain("Authorization");
			expect(out).not.toContain("import os");
			expect(out).not.toContain("PG_API_KEY");
		});

		it("node omits apiKey from the client", () => {
			const out = generateRoutingTestCommandNode(rule, opts);
			expect(out).not.toContain("apiKey");
			expect(out).not.toContain("PG_API_KEY");
		});

		it("go omits WithAPIKey and the os import", () => {
			const out = generateRoutingTestCommandGo(rule, opts);
			expect(out).not.toContain("WithAPIKey");
			expect(out).not.toContain('"os"');
			expect(out).not.toContain("PG_API_KEY");
		});
	});

	describe("when auth is enforced without a selected key", () => {
		const opts = { ...BASE, enforceAuth: true } as const;

		it("curl uses the CELER_ROUTE_API_KEY placeholder", () => {
			const out = generateRoutingTestCommandCurl(rule, opts);
			expect(out).toContain("Authorization: Bearer ${CELER_ROUTE_API_KEY}");
			expect(out).not.toContain("PG_API_KEY");
		});

		it("python references CELER_ROUTE_API_KEY from the environment", () => {
			const out = generateRoutingTestCommandPython(rule, opts);
			expect(out).toContain('os.environ["CELER_ROUTE_API_KEY"]');
			expect(out).not.toContain("PG_API_KEY");
		});

		it("node references process.env.CELER_ROUTE_API_KEY", () => {
			const out = generateRoutingTestCommandNode(rule, opts);
			expect(out).toContain("process.env.CELER_ROUTE_API_KEY");
			expect(out).not.toContain("PG_API_KEY");
		});

		it("go references os.Getenv(CELER_ROUTE_API_KEY)", () => {
			const out = generateRoutingTestCommandGo(rule, opts);
			expect(out).toContain('os.Getenv("CELER_ROUTE_API_KEY")');
			expect(out).not.toContain("PG_API_KEY");
		});
	});

	describe("when a key is selected", () => {
		const opts = { ...BASE, enforceAuth: true, vkValue: "sk-real" } as const;

		it("curl inlines the selected key", () => {
			const out = generateRoutingTestCommandCurl(rule, opts);
			expect(out).toContain("Authorization: Bearer sk-real");
		});

		it("python inlines the selected key", () => {
			const out = generateRoutingTestCommandPython(rule, opts);
			expect(out).toContain('"sk-real"');
		});

		it("node inlines the selected key", () => {
			const out = generateRoutingTestCommandNode(rule, opts);
			expect(out).toContain('"sk-real"');
		});

		it("go inlines the selected key", () => {
			const out = generateRoutingTestCommandGo(rule, opts);
			expect(out).toContain('"sk-real"');
		});
	});
});