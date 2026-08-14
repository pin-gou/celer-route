import { expect, test } from "../../core/fixtures/base.fixture";

test.describe("i18n Language Switching", () => {
  test.describe.configure({ mode: "serial" });

  test.beforeEach(async ({ sidebarPage }) => {
    // Navigate to the dashboard page to ensure we're logged in and on a known page
    await sidebarPage.page.goto("/workspace/dashboard");
    await sidebarPage.waitForPageLoad();
  });

  test("should display language switcher in the user menu", async ({
    sidebarPage,
  }) => {
    // The language switcher should be available in the user menu
    // This test will fail in the red phase because the LanguageSwitcher component
    // has not been implemented yet.
    const userMenuTrigger = sidebarPage.page.getByTestId(
      "language-switcher-trigger"
    );
    await expect(userMenuTrigger).toBeVisible({ timeout: 10000 });
  });

  test("should switch to zh-CN and display Chinese navigation labels", async ({
    sidebarPage,
  }) => {
    // Open the language switcher
    const languageTrigger = sidebarPage.page.getByTestId(
      "language-switcher-trigger"
    );
    await expect(languageTrigger).toBeVisible({ timeout: 10000 });
    await languageTrigger.click();

    // Select zh-CN option
    const zhOption = sidebarPage.page.getByTestId(
      "language-switcher-option-zh-CN"
    );
    await expect(zhOption).toBeVisible({ timeout: 5000 });
    await zhOption.click();

    // Wait for locale to switch and UI to re-render
    await sidebarPage.page.waitForTimeout(500);

    // Assert that navigation labels appear in Chinese
    // The sidebar navigation links should now show Chinese text
    const dashboardLink = sidebarPage.page.getByRole("link", {
      name: /仪表板/i,
    });
    await expect(dashboardLink).toBeVisible({ timeout: 10000 });

    // "模型提供商" is a sub-item of the collapsible "模型" (Models) section;
    // expand the section so the sub-item becomes visible.
    // Use the role-based locator; in zh-CN there is only one match.
    const modelsSection = sidebarPage.page.getByRole("button", {
      name: /^模型$/,
    });
    await expect(modelsSection).toBeVisible({ timeout: 5000 });
    await modelsSection.click();

    const providersLink = sidebarPage.page.getByRole("link", {
      name: /提供商/i,
    });
    await expect(providersLink).toBeVisible({ timeout: 5000 });

    // "治理" renders as a collapsible section toggle button (not a link)
    const governanceSection = sidebarPage.page.getByRole("button", {
      name: /^治理$/,
    });
    await expect(governanceSection).toBeVisible({ timeout: 5000 });

    // Check that the dashboard page title is in Chinese
    await expect(
      sidebarPage.page.getByRole("heading", { name: /仪表板/i })
    ).toBeVisible({ timeout: 5000 });
  });

  test("should switch back to en and restore English labels", async ({
    sidebarPage,
  }) => {
    // First switch to zh-CN
    const languageTrigger = sidebarPage.page.getByTestId(
      "language-switcher-trigger"
    );
    await expect(languageTrigger).toBeVisible({ timeout: 10000 });
    await languageTrigger.click();

    const zhOption = sidebarPage.page.getByTestId(
      "language-switcher-option-zh-CN"
    );
    await expect(zhOption).toBeVisible({ timeout: 5000 });
    await zhOption.click();

    await sidebarPage.page.waitForTimeout(500);

    // Now switch back to en
    await languageTrigger.click();
    await sidebarPage.page.waitForTimeout(300);

    const enOption = sidebarPage.page.getByTestId(
      "language-switcher-option-en"
    );
    await expect(enOption).toBeVisible({ timeout: 5000 });
    await enOption.click();

    // Wait for locale to switch back
    await sidebarPage.page.waitForTimeout(500);

    // Assert that navigation labels are back in English
    const dashboardLink = sidebarPage.page.getByRole("link", {
      name: /dashboard/i,
    });
    await expect(dashboardLink).toBeVisible({ timeout: 10000 });

    // Expand the "Models" section so "Model Providers" sub-item becomes visible.
    // Use .first() because the dashboard page also has a "Models" filter button.
    const modelsSection = sidebarPage.page.getByRole("button", {
      name: "Models",
    }).first();
    await expect(modelsSection).toBeVisible({ timeout: 5000 });
    await modelsSection.click();

    const providersLink = sidebarPage.page.getByRole("link", {
      name: /providers/i,
    });
    await expect(providersLink).toBeVisible({ timeout: 5000 });

    // Check that the dashboard page title is back in English
    await expect(
      sidebarPage.page.getByRole("heading", { name: /dashboard/i })
    ).toBeVisible({ timeout: 5000 });
  });

  test("should persist language preference across page reload", async ({
    sidebarPage,
  }) => {
    // Switch to zh-CN
    const languageTrigger = sidebarPage.page.getByTestId(
      "language-switcher-trigger"
    );
    await expect(languageTrigger).toBeVisible({ timeout: 10000 });
    await languageTrigger.click();

    const zhOption = sidebarPage.page.getByTestId(
      "language-switcher-option-zh-CN"
    );
    await expect(zhOption).toBeVisible({ timeout: 5000 });
    await zhOption.click();

    await sidebarPage.page.waitForTimeout(500);

    // Reload the page
    await sidebarPage.page.reload();
    await sidebarPage.waitForPageLoad();

    // After reload, the locale should still be zh-CN (persisted in localStorage)
    const dashboardLink = sidebarPage.page.getByRole("link", {
      name: /仪表板/i,
    });
    await expect(dashboardLink).toBeVisible({ timeout: 15000 });
  });

  test("should handle corrupted localStorage gracefully", async ({
    sidebarPage,
  }) => {
    // Corrupt the locale storage entry
    await sidebarPage.page.evaluate(() => {
      localStorage.setItem("bifrost.locale", "{invalid}");
    });

    // Reload the page — should not crash, should fallback to en
    await sidebarPage.page.reload();
    await sidebarPage.waitForPageLoad();

    // After reload with corrupted locale, the UI should fallback to English
    const dashboardLink = sidebarPage.page.getByRole("link", {
      name: /dashboard/i,
    });
    await expect(dashboardLink).toBeVisible({ timeout: 15000 });
  });

  // ─────────────────────────────────────────────────────────────────
  // 19-route traversal: switch to zh-CN, assert Chinese text, switch
  // back to en, assert English text.
  // Red phase: the 19 routes have not been translated yet, so the
  // Chinese text assertions will fail (expected TDD red behavior).
  // ─────────────────────────────────────────────────────────────────
  const ROUTES_TO_VERIFY = [
    {
      path: "/workspace/logs",
      zhTitle: /日志/i,
      enTitle: /logs/i,
    },
    {
      path: "/workspace/mcp-logs",
      zhTitle: /MCP 日志|mcp日志/i,
      enTitle: /mcp.?logs/i,
    },
    {
      path: "/workspace/mcp-registry",
      zhTitle: /MCP 注册|mcp注册/i,
      enTitle: /mcp.?registry/i,
    },
    {
      path: "/workspace/mcp-sessions",
      zhTitle: /MCP 会话|mcp会话/i,
      enTitle: /mcp.?sessions/i,
    },
    {
      path: "/workspace/model-limits",
      zhTitle: /模型限额|模型限制/i,
      enTitle: /model.?limits/i,
    },
    {
      path: "/workspace/routing-rules",
      zhTitle: /路由规则/i,
      enTitle: /routing.?rules/i,
    },
    {
      path: "/workspace/complexity-router",
      zhTitle: /复杂路由|复杂度路由/i,
      enTitle: /complexity.?router/i,
    },
    {
      path: "/workspace/skills-repo",
      zhTitle: /技能仓库|技能/i,
      enTitle: /skills/i,
    },
    {
      path: "/workspace/plugins",
      zhTitle: /插件/i,
      enTitle: /plugins/i,
    },
    {
      path: "/workspace/observability",
      zhTitle: /可观测性|观测/i,
      enTitle: /observability/i,
    },
    {
      path: "/workspace/webhooks",
      zhTitle: /Webhook|webhook/i,
      enTitle: /webhooks/i,
    },
    {
      path: "/workspace/oauth-grants",
      zhTitle: /OAuth 授权|oAuth授权/i,
      enTitle: /oauth.?grants/i,
    },
    {
      path: "/workspace/custom-pricing",
      zhTitle: /自定义定价|定价/i,
      enTitle: /custom.?pricing/i,
    },
    {
      path: "/workspace/mcp-settings",
      zhTitle: /MCP 设置|mcp设置/i,
      enTitle: /mcp.?settings/i,
    },
    {
      path: "/workspace/config",
      zhTitle: /配置/i,
      enTitle: /config/i,
    },
    {
      path: "/workspace/model-catalog",
      zhTitle: /模型目录|模型分类/i,
      enTitle: /model.?catalog/i,
    },
    {
      path: "/workspace/virtual-keys",
      zhTitle: /虚拟密钥|虚拟钥匙/i,
      enTitle: /virtual.?keys/i,
    },
    {
      path: "/workspace/docs",
      zhTitle: /文档/i,
      enTitle: /docs/i,
    },
    {
      path: "/workspace/prompt-repo",
      zhTitle: /提示仓库|提示词/i,
      enTitle: /prompt.?repo/i,
    },
  ];

  for (const route of ROUTES_TO_VERIFY) {
    test(`should switch to zh-CN and back to en on ${route.path}`, async ({
      sidebarPage,
    }) => {
      // Navigate to the route
      await sidebarPage.page.goto(route.path);
      await sidebarPage.waitForPageLoad();

      // Switch to zh-CN
      const languageTrigger = sidebarPage.page.getByTestId(
        "language-switcher-trigger"
      );
      await expect(languageTrigger).toBeVisible({ timeout: 10000 });
      await languageTrigger.click();

      const zhOption = sidebarPage.page.getByTestId(
        "language-switcher-option-zh-CN"
      );
      await expect(zhOption).toBeVisible({ timeout: 5000 });
      await zhOption.click();

      // Wait for re-render
      await sidebarPage.page.waitForTimeout(500);

      // Assert Chinese text appears
      // Red phase: the page has not been translated yet, so this assertion
      // will fail (expected TDD red behavior).
      await expect(
        sidebarPage.page.getByRole("heading", { name: route.zhTitle }).first(),
        `${route.path}: should display Chinese heading after switching to zh-CN (red phase — route not yet translated)`
      ).toBeVisible({ timeout: 10000 });

      // Switch back to en
      await languageTrigger.click();
      await sidebarPage.page.waitForTimeout(300);

      const enOption = sidebarPage.page.getByTestId(
        "language-switcher-option-en"
      );
      await expect(enOption).toBeVisible({ timeout: 5000 });
      await enOption.click();

      await sidebarPage.page.waitForTimeout(500);

      // Assert English text is restored
      await expect(
        sidebarPage.page.getByRole("heading", { name: route.enTitle }).first(),
        `${route.path}: should display English heading after switching back to en`
      ).toBeVisible({ timeout: 10000 });
    });
  }
});