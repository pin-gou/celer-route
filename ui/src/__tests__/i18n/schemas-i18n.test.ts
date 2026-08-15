/**
 * schemas-i18n.test.ts
 *
 * TDD Red Phase — 验证 Zod schema 校验错误消息走 i18n 翻译路径。
 *
 * design.md 契约：
 *   - ui/lib/types/schemas.ts 顶层 import i18n 实例（@/lib/i18n/config）
 *   - ~70+ 处硬编码 message 替换为 `t("validation.*")` 或
 *     `t("validation.fieldRequired", { field: ... })`
 *   - common.json 新增 `validation.*` 段，zh-CN 返回中文
 *
 * 本测试 mock `@/lib/i18n/config` 的 t() 返回固定中文，断言 schema 校验
 * 错误消息为中文（如 "端点为必填项"）而非英文（如 "Endpoint is required"）。
 *
 * 当前 schemas.ts 全部为硬编码英文（未接入 i18n），因此：
 *   - mock 已注入但 schemas 未消费 → 所有断言失败（红 phase ✓）
 * dev phase 2.2 接入 i18n 后，同一 mock 自动生效 → 转绿。
 */

import { describe, it, expect, vi, beforeEach } from "vitest";

// ─── Mock i18n 实例（@/lib/i18n/config 的 default 导出）────────────────
// schemas.ts 在 dev phase 将以 `import i18n from "@/lib/i18n/config"` 方式
// 获取 t()。vi.mock 挂在模块路径上，schemas 接入后自动拦截。
vi.mock("@/lib/i18n/config", () => {
  return {
    default: {
      t: (key: string, opts?: Record<string, unknown>) => {
        const field = typeof opts?.field === "string" ? opts.field : "";
        switch (key) {
          case "validation.fieldRequired":
            return `${field} 为必填项`;
          case "validation.urlInvalid":
            return "URL 格式无效";
          case "validation.minLength":
            return `至少 ${opts?.n ?? "?"} 个字符`;
          case "validation.maxLength":
            return `最多 ${opts?.n ?? "?"} 个字符`;
          case "validation.rangeOutOfBounds":
            return `${field} 必须在 ${opts?.min ?? "?"} 到 ${opts?.max ?? "?"} 之间`;
          default:
            return `[untranslated:${key}]`;
        }
      },
    },
  };
});

// schemas 需在 mock 之后 import（vi.mock 会被提升到顶部，这里顺序安全）
import {
  customProviderNameSchema,
  azureKeyConfigSchema,
  networkFormConfigSchema,
  vertexKeyConfigSchema,
  bedrockKeyConfigSchema,
  vllmKeyConfigSchema,
  ollamaKeyConfigSchema,
  proxyFormConfigSchema,
  s3BucketConfigSchema,
} from "@/lib/types/schemas";

describe("schemas.ts Zod 校验消息 i18n", () => {
  beforeEach(() => {
    // 确保每个用例独立验证，无 i18n 状态残留
    vi.clearAllMocks();
  });

  it("customProviderNameSchema.min(1, t(...)) 错误消息应含中文", () => {
    // 输入：空字符串 → trigger .min(1) 校验
    const result = customProviderNameSchema.safeParse("");

    expect(result.success).toBe(false);
    const message = result.error!.issues[0].message;
    // dev 后 message 应为 t("validation.fieldRequired", { field: "Custom provider name" }) → "Custom provider name 为必填项"
    expect(message).toBe("Custom provider name 为必填项");
  });

  it("azureKeyConfigSchema endpoint refine 错误消息应为中文（Endpoint 为必填项）", () => {
    // 输入：空对象 → endpoint 未设置 → refine 失败（path=["endpoint"]）
    const result = azureKeyConfigSchema.safeParse({});

    expect(result.success).toBe(false);
    const endpointIssue = result.error!.issues.find((i) => i.path[0] === "endpoint");
    expect(endpointIssue).toBeDefined();
    // dev 后 message 应为 t("validation.fieldRequired", { field: "Endpoint" }) → "Endpoint 为必填项"
    expect(endpointIssue!.message).toBe("Endpoint 为必填项");
    expect(endpointIssue!.message).not.toContain("required");
  });

  it("networkFormConfigSchema base_url 错误消息应为中文（URL 格式无效）", () => {
    // 输入：非法 URL → z.string().url(t("validation.urlInvalid")) 失败
    const result = networkFormConfigSchema.safeParse({
      base_url: "not-a-url",
      default_request_timeout_in_seconds: 30,
      max_retries: 1,
      retry_backoff_initial: 100,
      retry_backoff_max: 1000,
    });

    expect(result.success).toBe(false);
    const messages = (result.error?.issues ?? []).map((i) => i.message);
    // dev 后必须存在中文 URL 错误消息（z.config customError 或 .url() 内联 message）
    expect(messages.some((m) => m.includes("URL 格式无效"))).toBe(true);
  });

  it("vertexKeyConfigSchema refine 错误消息应为中文（Project ID 为必填项）", () => {
    const result = vertexKeyConfigSchema.safeParse({});
    expect(result.success).toBe(false);
    const projectIdIssue = result.error!.issues.find((i) => i.path[0] === "project_id");
    expect(projectIdIssue).toBeDefined();
    expect(projectIdIssue!.message).toBe("Project ID 为必填项");
  });

  it("bedrockKeyConfigSchema refine 错误消息应为中文（Region 为必填项）", () => {
    const result = bedrockKeyConfigSchema.safeParse({});
    expect(result.success).toBe(false);
    const regionIssue = result.error!.issues.find((i) => i.path[0] === "region");
    expect(regionIssue).toBeDefined();
    expect(regionIssue!.message).toBe("Region 为必填项");
  });

  it("vllmKeyConfigSchema model_name .min(1) 错误消息应为中文（Model name 为必填项）", () => {
    // vllmKeyConfigSchema is an object — parse { model_name: "" } to trigger .min(1) on model_name
    const result = vllmKeyConfigSchema.safeParse({ model_name: "" });
    expect(result.success).toBe(false);
    const modelNameIssue = result.error!.issues.find((i) => i.path[0] === "model_name");
    expect(modelNameIssue).toBeDefined();
    expect(modelNameIssue!.message).toBe("Model name 为必填项");
  });

  it("ollamaKeyConfigSchema url refine 错误消息应为中文（Server URL 为必填项）", () => {
    const result = ollamaKeyConfigSchema.safeParse({});
    expect(result.success).toBe(false);
    const urlIssue = result.error!.issues.find((i) => i.path[0] === "url");
    expect(urlIssue).toBeDefined();
    expect(urlIssue!.message).toBe("Server URL 为必填项");
  });

  it("s3BucketConfigSchema bucket_name .min(1) 错误消息应为中文", () => {
    const result = s3BucketConfigSchema.safeParse({ bucket_name: "", prefix: "" });
    expect(result.success).toBe(false);
    const bucketIssue = result.error!.issues.find((i) => i.path[0] === "bucket_name");
    expect(bucketIssue).toBeDefined();
    expect(bucketIssue!.message).toBe("Bucket name 为必填项");
  });

  it("proxyFormConfigSchema .url() refine 错误消息应为中文（URL 格式无效）", () => {
    // proxyFormConfigSchema has url: secretVarSchema.optional() — pass a non-URL value
    const result = proxyFormConfigSchema.safeParse({
      type: "http",
      url: { type: "plain_text", value: "not-a-url" },
    });
    expect(result.success).toBe(false);
    const urlIssue = result.error!.issues.find((i) => i.path[0] === "url");
    expect(urlIssue).toBeDefined();
    expect(urlIssue!.message).toBe("URL 格式无效");
  });

  it("有值输入应通过校验（反向控制：翻译接入不改变有效语义）", () => {
    const result = customProviderNameSchema.safeParse("my-provider");
    expect(result.success).toBe(true);
  });
});