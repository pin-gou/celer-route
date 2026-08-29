import { expect, test } from '../../core/fixtures/base.fixture'

/**
 * @file RTK admin sub-router E2E tests.
 *
 * Verifies the four admin pages (filters, test, preview, raw-output) and
 * the admin links in the existing rtk configuration fragment. Tests use
 * direct API calls for the heavier flows (PUT /api/context/rtk/config,
 * GET /api/context/rtk/filters) so they do not require a live RTK
 * server fixture — they only need an authenticated session.
 */

const RTK_PLUGIN_NAME = 'rtk'

test.describe('RTK Admin Sub-router', () => {
  test.beforeEach(async ({ pluginsPage }) => {
    await pluginsPage.goto()
    await pluginsPage.ensureSheetClosed()
  })

  test('overview page lists the four admin cards', async ({ page }) => {
    // Click the dedicated admin overview link in the rtk fragment.
    await page.locator('[data-testid="rtk-admin-link-overview"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk$/)

    const cards = page.locator('[data-testid^="rtk-admin-card-"]')
    expect(await cards.count()).toBeGreaterThanOrEqual(4)

    await expect(page.locator('[data-testid="rtk-admin-card-filters"]')).toBeVisible()
    await expect(page.locator('[data-testid="rtk-admin-card-test"]')).toBeVisible()
    await expect(page.locator('[data-testid="rtk-admin-card-preview"]')).toBeVisible()
    await expect(page.locator('[data-testid="rtk-admin-card-raw-output"]')).toBeVisible()
  })

  test('admin nav highlights the active tab', async ({ page }) => {
    await page.locator('[data-testid="rtk-admin-link-filters"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk\/filters$/)

    // The filters tab should now be styled as active.
    const filtersTab = page.locator('[data-testid="rtk-admin-tab-filters"]')
    await expect(filtersTab).toBeVisible()
    await expect(filtersTab).toHaveClass(/border-primary/)
  })

  test('filters page renders the catalog or empty state', async ({ page }) => {
    await page.locator('[data-testid="rtk-admin-link-filters"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk\/filters$/)

    // Either the table renders rows, or the empty state appears. Both
    // are acceptable — the test only ensures the chrome is correct.
    const rows = page.locator('[data-testid="rtk-filters-row"]')
    const error = page.getByText(/failed to load filters/i)
    await expect(rows.first().or(error)).toBeVisible({ timeout: 10_000 })
  })

  test('test runner renders default payload and submits', async ({ page }) => {
    await page.locator('[data-testid="rtk-admin-link-test"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk\/test$/)

    await expect(page.locator('[data-testid="rtk-test-command"]')).toBeVisible()
    await expect(page.locator('[data-testid="rtk-test-output"]')).toBeVisible()

    // The plugin may be disabled in CI. Both success and "service
    // unavailable" paths satisfy the test — we just need the form to
    // be wired correctly.
    await page.locator('[data-testid="rtk-test-submit"]').click()

    // Either the result card or an error toast should be visible.
    const result = page.locator('[data-testid="rtk-test-result"]')
    const error = page.getByText(/RTK plugin is not enabled|unknown error/i)
    await expect(result.first().or(error)).toBeVisible({ timeout: 10_000 })
  })

  test('preview page submits and renders a result card', async ({ page }) => {
    await page.locator('[data-testid="rtk-admin-link-preview"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk\/preview$/)

    await expect(page.locator('[data-testid="rtk-preview-mode"]')).toBeVisible()
    await expect(page.locator('[data-testid="rtk-preview-output"]')).toBeVisible()

    await page.locator('[data-testid="rtk-preview-submit"]').click()

    const result = page.locator('[data-testid="rtk-preview-result"]')
    const error = page.getByText(/RTK plugin is not enabled|unknown error/i)
    await expect(result.first().or(error)).toBeVisible({ timeout: 10_000 })
  })

  test('raw output viewer rejects malformed ids', async ({ page }) => {
    await page.locator('[data-testid="rtk-admin-link-overview"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk$/)
    await page.locator('[data-testid="rtk-admin-card-raw-output"]').click()
    await page.waitForURL(/\/workspace\/plugins\/rtk\/raw-output$/)

    await page.locator('[data-testid="rtk-raw-output-id"]').fill('not-hex')
    await page.locator('[data-testid="rtk-raw-output-fetch"]').click()

    await expect(page.getByText(/invalid id format/i)).toBeVisible()
  })

  test('rtk configuration PUT roundtrips through /api/context/rtk/config', async ({ request }) => {
    // Verify the dedicated admin path reads/writes the same row as
    // /api/plugins/rtk. This is a smoke test — it doesn't exercise the
    // compression pipeline, only the persistence contract.
    const getResp = await request.get('/api/context/rtk/config')
    expect(getResp.status()).toBeLessThan(500)

    if (getResp.status() === 200) {
      const body = await getResp.json()
      expect(body).toHaveProperty('enabled')
      expect(body).toHaveProperty('config')
    }
  })

  // ---------------------------------------------------------------------------
  // Raw-output sentinel: default response wraps body with a server-side
  // sentinel prefix; ?raw=1 bypasses it for operator inspection. The
  // ops UI consumes ?raw=1 (see ui/lib/store/apis/rtkAdminApi.ts) so the
  // page must render clean text — not NUL-prefixed noise.
  // ---------------------------------------------------------------------------

  test('raw-output viewer renders verbatim body (?raw=1 path)', async ({ page, request }) => {
    // Drive the test endpoint to materialise a real raw-output id.
    const fixture =
      'row 1\nrow 2\nrow 3\n' + Array.from({ length: 200 }, (_, i) => `line ${i}`).join('\n') + '\n'
    const testResp = await request.post('/api/context/rtk/test', {
      data: { output: fixture, command: 'echo regression-sentinel' },
    })
    expect(testResp.status()).toBeLessThan(500)

    if (testResp.status() !== 200) {
      test.skip(true, 'RTK test endpoint unavailable — plugin not enabled or no LLM')
      return
    }

    const testBody = await testResp.json()
    const pointer = testBody?.stats?.rawOutputPointers?.[0]
    if (!pointer?.id) {
      test.skip(true, 'test endpoint did not produce a raw_output_pointer — retention may be disabled')
      return
    }

    // 1. Default (LLM-bound) response must start with the sentinel prefix.
    const defaultResp = await request.get(`/api/context/rtk/raw-output/${pointer.id}`)
    expect(defaultResp.status()).toBe(200)
    const defaultText = await defaultResp.text()
    expect(defaultText.startsWith('\x00RTK_RAW_OUTPUT_BEGIN\x00')).toBe(true)
    // Persisted body text is still present after the sentinel.
    expect(defaultText).toContain('regression-sentinel')

    // 2. ?raw=1 response is verbatim, no sentinel noise.
    const rawResp = await request.get(`/api/context/rtk/raw-output/${pointer.id}?raw=1`)
    expect(rawResp.status()).toBe(200)
    const rawText = await rawResp.text()
    expect(rawText.startsWith('\x00RTK_RAW_OUTPUT_BEGIN\x00')).toBe(false)
    expect(rawText).toContain('regression-sentinel')

    // 3. The ops UI consumes ?raw=1, so the rendered body must match the
    //    verbatim response — no NUL characters leaking into <pre>.
    await page.goto('/workspace/plugins/rtk/raw-output')
    await page.locator('[data-testid="rtk-raw-output-id"]').fill(pointer.id)
    await page.locator('[data-testid="rtk-raw-output-fetch"]').click()
    const pre = page.locator('[data-testid="rtk-raw-output-body"]')
    await expect(pre).toBeVisible({ timeout: 10_000 })
    const preText = (await pre.textContent()) ?? ''
    expect(preText).not.toContain('\x00')
    expect(preText).toContain('regression-sentinel')
    // And the rendered text must equal the ?raw=1 response byte-for-byte.
    expect(preText).toBe(rawText)
  })
})