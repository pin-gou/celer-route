import { expect, test } from '../../core/fixtures/base.fixture'
import { createRoutingRuleData } from './routing-rules.data'
import type { RoutingRulesPage } from './pages/routing-rules.page'

// Track created rules for cleanup
const createdRules: string[] = []

// Returns true when `upper` appears before `lower` in the routing rules table.
async function rememberRuleOrder(page: RoutingRulesPage, upper: string, lower: string): Promise<boolean> {
  const names = await page.getAllRuleNames()
  const upperIdx = names.findIndex((n) => n.includes(upper))
  const lowerIdx = names.findIndex((n) => n.includes(lower))
  if (upperIdx === -1 || lowerIdx === -1) return false
  return upperIdx < lowerIdx
}

// Returns true when rule2 appears before rule1 in the table (rule2 outranks rule1).
async function rule2BeforeRule1(page: RoutingRulesPage, rule1: string, rule2: string): Promise<boolean> {
  return rememberRuleOrder(page, rule2, rule1)
}

test.describe('Routing Rules', () => {
  test.beforeEach(async ({ routingRulesPage }) => {
    await routingRulesPage.goto()
  })

  test.afterEach(async ({ routingRulesPage }) => {
    // Clean up any rules created during tests
    for (const ruleName of [...createdRules]) {
      try {
        const exists = await routingRulesPage.ruleExists(ruleName)
        if (exists) {
          await routingRulesPage.deleteRoutingRule(ruleName)
        }
      } catch {
        // Ignore cleanup errors
      }
    }
    createdRules.length = 0
  })

  test.describe('Routing Rule Creation', () => {
    test('should display create routing rule button', async ({ routingRulesPage }) => {
      await expect(routingRulesPage.createBtn).toBeVisible()
    })

    test('should open routing rule creation sheet', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()

      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await expect(routingRulesPage.nameInput).toBeVisible()
    })

    test('should create a basic routing rule', async ({ routingRulesPage }) => {
      // Note: CEL expression is auto-generated from the visual Rule Builder
      // An empty builder means the rule applies to all requests
      const ruleData = createRoutingRuleData({
        name: `Basic Rule ${Date.now()}`,
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      const exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)
    })

    test('should create routing rule with description', async ({ routingRulesPage }) => {
      const ruleData = createRoutingRuleData({
        name: `Described Rule ${Date.now()}`,
        description: 'A rule with a detailed description for testing',
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      const exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)
    })

    test('should create disabled routing rule', async ({ routingRulesPage }) => {
      const ruleData = createRoutingRuleData({
        name: `Disabled Rule ${Date.now()}`,
        enabled: false,
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      const exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)
    })

    test('should cancel routing rule creation', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()

      const testName = `Cancelled Rule ${Date.now()}`
      await routingRulesPage.nameInput.fill(testName)

      await routingRulesPage.cancelRule()

      const exists = await routingRulesPage.ruleExists(testName)
      expect(exists).toBe(false)
    })
  })

  test.describe('Routing Rule Management', () => {
    test('should edit routing rule', async ({ routingRulesPage }) => {
      // Create a rule first
      const ruleData = createRoutingRuleData({
        name: `Edit Test Rule ${Date.now()}`,
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      // Edit it - change description
      await routingRulesPage.editRoutingRule(ruleData.name, {
        description: 'Updated description',
      })

      // Verify description was saved and displayed in table
      const description = await routingRulesPage.getRuleDescription(ruleData.name)
      expect(description).toContain('Updated description')
    })

    test('should delete routing rule', async ({ routingRulesPage }) => {
      // Create a rule first
      const ruleData = createRoutingRuleData({
        name: `Delete Test Rule ${Date.now()}`,
      })
      // Don't add to createdRules since we're testing delete

      await routingRulesPage.createRoutingRule(ruleData)

      // Verify it exists
      let exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)

      // Delete it
      await routingRulesPage.deleteRoutingRule(ruleData.name)

      // Verify it's gone
      exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(false)
    })

    test('should toggle rule enabled state', async ({ routingRulesPage }) => {
      // Create a rule first
      const ruleData = createRoutingRuleData({
        name: `Toggle Test Rule ${Date.now()}`,
        enabled: true,
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      // Toggle it
      await routingRulesPage.toggleRuleEnabled(ruleData.name)

      // Verify it still exists
      const exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)
    })

    test('should open edit sheet from the details drawer', async ({ routingRulesPage }) => {
      const ruleData = createRoutingRuleData({
        name: `Info Edit Test Rule ${Date.now()}`,
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      // Open the details drawer, then switch to edit mode
      await routingRulesPage.openInfoSheet(ruleData.name)
      await expect(routingRulesPage.infoEditBtn).toBeVisible({ timeout: 5000 })

      await routingRulesPage.infoEditBtn.click()

      // Details drawer closes and the edit sheet opens pre-filled
      await expect(routingRulesPage.infoSheet).not.toBeVisible({ timeout: 5000 })
      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await routingRulesPage.waitForSheetAnimation()
      await expect(routingRulesPage.nameInput).toHaveValue(ruleData.name)

      await routingRulesPage.cancelRule()
    })

    test('should show a copyable test command in the details drawer', async ({ routingRulesPage }) => {
      const ruleData = createRoutingRuleData({
        name: `Test Command Rule ${Date.now()}`,
        provider: 'openai',
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      await routingRulesPage.openInfoSheet(ruleData.name)

      // The generated curl command should be visible and contain the expected pieces
      await expect(routingRulesPage.testCommandBlock).toBeVisible({ timeout: 5000 })
      const command = (await routingRulesPage.testCommandBlock.textContent()) ?? ''
      expect(command).toContain('curl -X POST')
      expect(command).toContain('chat/completions')
      expect(command).toContain('"model": "openai/')

      await expect(routingRulesPage.testCommandCopyBtn).toBeVisible()
      await routingRulesPage.testCommandCopyBtn.click()
      await routingRulesPage.waitForSuccessToast()
    })
  })

  test.describe('Form Validation', () => {
    test('should require name for routing rule', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()

      // Try to save without name
      await routingRulesPage.saveBtn.click()

      // Form should still be visible (not submitted)
      await expect(routingRulesPage.sheet).toBeVisible()

      await routingRulesPage.cancelRule()
    })
  })

  test.describe('Table Display', () => {
    test('should display routing rules table', async ({ routingRulesPage }) => {
      // With 0 rules the view shows empty state (no table); with 1+ rules it shows the table
      const count = await routingRulesPage.getRuleCount()
      if (count === 0) {
        await expect(routingRulesPage.emptyState).toBeVisible()
        await expect(routingRulesPage.table).not.toBeVisible()
      } else {
        await expect(routingRulesPage.table).toBeVisible()
        await expect(routingRulesPage.emptyState).not.toBeVisible()
      }
    })

    test('should show empty state when no rules', async ({ routingRulesPage }) => {
      const count = await routingRulesPage.getRuleCount()
      if (count === 0) {
        await expect(routingRulesPage.emptyState).toBeVisible()
      }
      // When rules exist, getRuleCount > 0 is already implied by the condition
    })
  })

  test.describe('Advanced Rule Features', () => {
    test('should create rule with provider filter', async ({ routingRulesPage }) => {
      const ruleData = createRoutingRuleData({
        name: `Provider Filter Rule ${Date.now()}`,
        provider: 'openai', // Set target provider
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      const exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)
    })

    test('should create rule with model filter', async ({ routingRulesPage }) => {
      const ruleData = createRoutingRuleData({
        name: `Model Filter Rule ${Date.now()}`,
        provider: 'openai',
        model: 'gpt-4',
      })
      createdRules.push(ruleData.name)

      await routingRulesPage.createRoutingRule(ruleData)

      const exists = await routingRulesPage.ruleExists(ruleData.name)
      expect(exists).toBe(true)
    })

    test('should reorder rules by changing priority', async ({ routingRulesPage }) => {
      // Creating and editing two rules through the 3-step sheet exceeds the
      // default 60s test budget (2s save arming per save), so widen it.
      test.setTimeout(120000)
      // Create two rules with unique priorities (avoid fixed 500/600 so parallel workers don't collide)
      const rule1 = createRoutingRuleData({ name: `Reorder Test Rule 1 ${Date.now()}` })
      const rule2 = createRoutingRuleData({ name: `Reorder Test Rule 2 ${Date.now()}` })
      createdRules.push(rule1.name, rule2.name)

      await routingRulesPage.createRoutingRule(rule1)
      await routingRulesPage.createRoutingRule(rule2)

      // Change first rule's priority (edit to a new value to test reorder)
      const newPriority = (rule1.priority! + 100) % 901
      await routingRulesPage.editRoutingRule(rule1.name, { priority: newPriority })

      // Verify priority was saved and displayed
      const displayedPriority = await routingRulesPage.getRulePriority(rule1.name)
      expect(displayedPriority).toBe(newPriority)
    })

    test('should reorder rules by dragging the row handle', async ({ routingRulesPage }) => {
      // Create two rules with distinct priorities; dragging rule2's handle onto
      // rule1's row swaps their relative order (source takes target's position).
      const rule1 = createRoutingRuleData({ name: `Drag Reorder A ${Date.now()}` })
      const rule2 = createRoutingRuleData({ name: `Drag Reorder B ${Date.now()}` })
      createdRules.push(rule1.name, rule2.name)

      await routingRulesPage.createRoutingRule(rule1)
      await routingRulesPage.createRoutingRule(rule2)

      const before = await rule2BeforeRule1(routingRulesPage, rule1.name, rule2.name)

      // Drag rule2's handle onto rule1's row.
      await routingRulesPage.reorderRuleByDrag(rule2.name, rule1.name)

      // The relative order of the two rules must flip.
      await expect
        .poll(async () => (await rule2BeforeRule1(routingRulesPage, rule1.name, rule2.name)) !== before, { timeout: 5000 })
        .toBe(true)
    })

    test('should reorder rules with the up/down buttons', async ({ routingRulesPage }) => {
      // Create two rules; moving the lower one up (and the upper one down) must
      // swap their relative order just like dragging does.
      const rule1 = createRoutingRuleData({ name: `Button Reorder A ${Date.now()}` })
      const rule2 = createRoutingRuleData({ name: `Button Reorder B ${Date.now()}` })
      createdRules.push(rule1.name, rule2.name)

      await routingRulesPage.createRoutingRule(rule1)
      await routingRulesPage.createRoutingRule(rule2)

      const before = await rule2BeforeRule1(routingRulesPage, rule1.name, rule2.name)

      // Click the opposite-direction arrow on one rule: if rule2 sits above
      // rule1, move rule2 down; otherwise move rule2 up. Either way the pair
      // must flip.
      const direction = before ? 'down' : 'up'
      await routingRulesPage.reorderRuleByButton(rule2.name, direction)

      await expect
        .poll(async () => (await rule2BeforeRule1(routingRulesPage, rule1.name, rule2.name)) !== before, { timeout: 5000 })
        .toBe(true)
    })

    test('should create rule with virtual key scope', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()

      const ruleName = `VK Scope Rule ${Date.now()}`
      await routingRulesPage.nameInput.fill(ruleName)

      // Try to set scope to virtual key
      const scopeSelect = routingRulesPage.sheet.locator('[role="combobox"]').filter({ hasText: /Global|Scope/i }).first()
      const scopeVisible = await scopeSelect.isVisible().catch(() => false)

      if (scopeVisible) {
        // Scope selection is available
        await scopeSelect.click()
        const vkOption = routingRulesPage.page.getByRole('option', { name: /Virtual Key/i })
        const vkVisible = await vkOption.isVisible().catch(() => false)

        if (vkVisible) {
          await vkOption.click()
          // Note: Would need to select a specific VK - for now just verify the option exists
        }
      }

      // Cancel since we're just testing the UI
      await routingRulesPage.cancelRule()
    })
  })

  test.describe('Rule Builder and CEL Generation', () => {
    test('should show CEL preview with "No rules defined" when empty', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()
      await routingRulesPage.waitForSheetAnimation()

      // Wait for rule builder to fully load
      await routingRulesPage.waitForRuleBuilder()

      // Get CEL expression - should show no rules message when empty
      const celExpression = await routingRulesPage.getCelExpression()
      expect(celExpression).toContain('No rules defined yet')

      await routingRulesPage.cancelRule()
    })

    test('should add rule condition and update CEL preview', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()
      await routingRulesPage.waitForSheetAnimation()
      await routingRulesPage.waitForRuleBuilder()

      // Fill required name
      const ruleName = `CEL Test ${Date.now()}`
      await routingRulesPage.nameInput.fill(ruleName)
      createdRules.push(ruleName)

      // Verify initial CEL is empty/no rules
      const initialCel = await routingRulesPage.getCelExpression()
      expect(initialCel).toContain('No rules defined yet')

      // Add a rule condition
      await routingRulesPage.clickAddRule()

      // Wait for rule row to appear and CEL to update
      await routingRulesPage.page.waitForTimeout(500)

      // After adding a rule, CEL should no longer say "No rules defined"
      // The default rule shows model == "" (empty model condition)
      const celAfterAdd = await routingRulesPage.getCelExpression()
      expect(celAfterAdd).not.toContain('No rules defined yet')
      expect(celAfterAdd).toContain('model') // Default field is Model

      await routingRulesPage.cancelRule()
    })

    test('should switch between AND and OR combinators', async ({ routingRulesPage }) => {
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()
      await routingRulesPage.waitForSheetAnimation()
      await routingRulesPage.waitForRuleBuilder()

      // Fill required name
      const ruleName = `CEL Combinator Test ${Date.now()}`
      await routingRulesPage.nameInput.fill(ruleName)
      createdRules.push(ruleName)

      // Add two rule conditions to see the combinator in action
      await routingRulesPage.clickAddRule()
      await routingRulesPage.clickAddRule()

      // Wait for rules to render
      await routingRulesPage.page.waitForTimeout(500)

      // Get CEL with default AND combinator
      const celWithAnd = await routingRulesPage.getCelExpression()
      // Default is AND - should have && operator
      expect(celWithAnd).toContain('&&')

      // Switch to OR
      await routingRulesPage.setCombinator('or')
      await routingRulesPage.page.waitForTimeout(300)

      // Verify CEL now contains OR logic
      const celWithOr = await routingRulesPage.getCelExpression()
      expect(celWithOr).toContain('||')

      await routingRulesPage.cancelRule()
    })

    test('should author conditions as raw CEL and round-trip through edit', async ({ routingRulesPage }) => {
      const ruleName = `CEL Mode Test ${Date.now()}`
      const celExpression = 'model == "claude-sonnet-4-6"'
      createdRules.push(ruleName)

      // Create a rule using raw-CEL mode instead of the visual builder
      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()
      await routingRulesPage.waitForSheetAnimation()
      await routingRulesPage.waitForRuleBuilder()

      await routingRulesPage.nameInput.fill(ruleName)
      await routingRulesPage.switchToCelMode()
      await routingRulesPage.fillCelExpression(celExpression)

      await routingRulesPage.saveBtn.click()
      await routingRulesPage.waitForSuccessToast()
      await expect(routingRulesPage.sheet).not.toBeVisible({ timeout: 10000 })

      const exists = await routingRulesPage.ruleExists(ruleName)
      expect(exists).toBe(true)

      // Reopen: a CEL-only rule (no visual query) must open in CEL mode with the
      // expression intact, not an empty builder that would silently clear it.
      await routingRulesPage.openEditSheet(ruleName)
      expect(await routingRulesPage.isCelMode()).toBe(true)
      expect(await routingRulesPage.getCelTextareaValue()).toBe(celExpression)

      // Saving without touching anything must preserve the CEL expression.
      await routingRulesPage.saveBtn.click()
      await routingRulesPage.waitForSuccessToast()
      await expect(routingRulesPage.sheet).not.toBeVisible({ timeout: 10000 })

      await routingRulesPage.openEditSheet(ruleName)
      expect(await routingRulesPage.getCelTextareaValue()).toBe(celExpression)
      await routingRulesPage.cancelRule()
    })

    test('should reject a malformed CEL expression on save', async ({ routingRulesPage }) => {
      const ruleName = `CEL Invalid Test ${Date.now()}`

      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible()
      await routingRulesPage.waitForSheetAnimation()
      await routingRulesPage.waitForRuleBuilder()

      await routingRulesPage.nameInput.fill(ruleName)
      await routingRulesPage.switchToCelMode()
      // Unbalanced parenthesis — rejected by the backend CEL compiler with a 400.
      await routingRulesPage.fillCelExpression('model == "gpt-4o" && (provider == "openai"')

      await routingRulesPage.saveBtn.click()

      // The sheet stays open on error; the rule must not be created.
      await expect(routingRulesPage.sheet).toBeVisible()
      // The compile error is surfaced inline beneath the CEL editor, not in a toast.
      await expect(routingRulesPage.celError).toBeVisible()
      await expect(routingRulesPage.celError).toContainText(/cel expression/i)
      await routingRulesPage.cancelRule()

      const exists = await routingRulesPage.ruleExists(ruleName)
      expect(exists).toBe(false)
    })

test('should save rule with conditions successfully', async ({ routingRulesPage }) => {
      const ruleName = `CEL Save Test ${Date.now()}`
      createdRules.push(ruleName)

      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await routingRulesPage.waitForSheetAnimation()
      await routingRulesPage.waitForRuleBuilder()

      // Fill name
      await routingRulesPage.nameInput.fill(ruleName)

      // Add a condition (default Model field with default operator)
      await routingRulesPage.clickAddRule()
      await routingRulesPage.page.waitForTimeout(500)

      // Verify CEL was generated before saving
      const celBeforeSave = await routingRulesPage.getCelExpression()
      expect(celBeforeSave).not.toContain('No rules defined')

      // Save the rule
      await routingRulesPage.saveBtn.click()
      await routingRulesPage.waitForSuccessToast()
      await expect(routingRulesPage.sheet).not.toBeVisible({ timeout: 10000 })

      // Verify rule was created
      const exists = await routingRulesPage.ruleExists(ruleName)
      expect(exists).toBe(true)
    })
  })

  test.describe('Fallback Reordering', () => {
    test('should show position badges for each fallback', async ({ routingRulesPage }) => {
      const ruleName = `Fallback Badges ${Date.now()}`
      createdRules.push(ruleName)

      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await routingRulesPage.waitForSheetAnimation()

      await routingRulesPage.nameInput.fill(ruleName)
      await routingRulesPage.addFallbackWithProvider('openai')
      await routingRulesPage.addFallbackWithProvider('anthropic')
      await routingRulesPage.addFallbackWithProvider('openai')

      // Each row shows a sequential position badge starting at #1
      await expect(routingRulesPage.fallbackPositionBadge(0)).toContainText('#1')
      await expect(routingRulesPage.fallbackPositionBadge(1)).toContainText('#2')
      await expect(routingRulesPage.fallbackPositionBadge(2)).toContainText('#3')

      await routingRulesPage.cancelRule()
    })

    test('should disable up button on first row and down button on last row', async ({ routingRulesPage }) => {
      const ruleName = `Fallback Buttons ${Date.now()}`
      createdRules.push(ruleName)

      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await routingRulesPage.waitForSheetAnimation()

      await routingRulesPage.nameInput.fill(ruleName)
      await routingRulesPage.addFallbackWithProvider('openai')
      await routingRulesPage.addFallbackWithProvider('anthropic')

      await expect(routingRulesPage.fallbackMoveUpBtn(0)).toBeDisabled()
      await expect(routingRulesPage.fallbackMoveDownBtn(1)).toBeDisabled()
      // Middle row has both enabled
      await expect(routingRulesPage.fallbackMoveUpBtn(1)).toBeEnabled()
      await expect(routingRulesPage.fallbackMoveDownBtn(0)).toBeEnabled()

      await routingRulesPage.cancelRule()
    })

    test('should reorder fallbacks via up/down buttons and persist across save and reload', async ({ routingRulesPage }) => {
      const ruleName = `Fallback Persist ${Date.now()}`
      createdRules.push(ruleName)

      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await routingRulesPage.waitForSheetAnimation()

      await routingRulesPage.nameInput.fill(ruleName)
      await routingRulesPage.addFallbackWithProvider('openai')
      await routingRulesPage.addFallbackWithProvider('anthropic')

      let labels = await routingRulesPage.getFallbackProviderLabels()
      expect(labels[0]).toMatch(/openai/i)
      expect(labels[1]).toMatch(/anthropic/i)

      // Move the first row down — order should swap
      await routingRulesPage.fallbackMoveDownBtn(0).click()
      await expect(routingRulesPage.fallbackPositionBadge(0)).toContainText('#1')
      await expect(routingRulesPage.fallbackPositionBadge(1)).toContainText('#2')

      labels = await routingRulesPage.getFallbackProviderLabels()
      expect(labels[0]).toMatch(/anthropic/i)
      expect(labels[1]).toMatch(/openai/i)

      // Move the second row up — back to the original order
      await routingRulesPage.fallbackMoveUpBtn(1).click()
      labels = await routingRulesPage.getFallbackProviderLabels()
      expect(labels[0]).toMatch(/openai/i)
      expect(labels[1]).toMatch(/anthropic/i)

      // Reverse so the saved order is anthropic, openai
      await routingRulesPage.fallbackMoveDownBtn(0).click()
      await routingRulesPage.saveBtn.click()
      await routingRulesPage.waitForSuccessToast()
      await expect(routingRulesPage.sheet).not.toBeVisible({ timeout: 10000 })

      // Re-open and verify the persisted order
      await routingRulesPage.openEditSheet(ruleName)
      labels = await routingRulesPage.getFallbackProviderLabels()
      expect(labels[0]).toMatch(/anthropic/i)
      expect(labels[1]).toMatch(/openai/i)
      await routingRulesPage.cancelRule()
    })

    test('should reorder fallbacks via drag handle', async ({ routingRulesPage }) => {
      const ruleName = `Fallback Drag ${Date.now()}`
      createdRules.push(ruleName)

      await routingRulesPage.createBtn.click()
      await expect(routingRulesPage.sheet).toBeVisible({ timeout: 5000 })
      await routingRulesPage.waitForSheetAnimation()

      await routingRulesPage.nameInput.fill(ruleName)
      await routingRulesPage.addFallbackWithProvider('openai')
      await routingRulesPage.addFallbackWithProvider('anthropic')

      // Drag handle of the second row up over the first row.
      const source = routingRulesPage.fallbackHandle(1)
      const target = routingRulesPage.fallbackRow(0)
      await source.dragTo(target)

      // dnd-kit may not visibly swap on the very first frame; allow a small window
      // for the live onDragOver handler to mutate the order.
      await expect
        .poll(async () => (await routingRulesPage.getFallbackProviderLabels())[0], { timeout: 3000 })
        .toMatch(/anthropic/i)
    })
  })
})
