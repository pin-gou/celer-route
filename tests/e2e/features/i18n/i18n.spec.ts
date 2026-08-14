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
});