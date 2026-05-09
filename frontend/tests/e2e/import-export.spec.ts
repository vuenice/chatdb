import { test, expect } from '@playwright/test'

test.describe('Import/Export functionality', () => {
  // Use API to get a valid token for testing 
  test.beforeEach(async ({ page }) => {
    // First, check if we can access the workbench - if auth required, redirect to login
    await page.goto('http://localhost:5173/workbench/import')
    await page.waitForLoadState('networkidle')
    
    // Wait for either workbench or login page 
    try {
      await page.waitForURL('**/workbench**', { timeout: 5000 })
    } catch {
      // On login page - fill and submit
      const select = page.locator('select')
      if (await select.isVisible({ timeout: 3000 }).catch(() => false)) {
        await select.selectOption({ index: 0 })
        await page.fill('input[type="text"]', 'postgres')
        await page.fill('input[type="password"]', 'root')
        await page.click('button[type="submit"]')
        await page.waitForURL('**/workbench**', { timeout: 10000 }).catch(() => {})
      }
    }
  })

  test('should display import page with all required UI elements', async ({ page }) => {
    await page.goto('http://localhost:5173/workbench/import')
    
    // Should have connection selector (via v-model)
    await expect(page.locator('select.io-input').first()).toBeVisible()
    
    // Should have import format buttons (psql/pgdump) for PostgreSQL
    await expect(page.locator('button.format-card:has-text("Plain SQL")')).toBeVisible()
    
    // Should have file input
    await expect(page.locator('input[type="file"]')).toBeVisible()
    
    // Should have import button
    await expect(page.locator('button.import-btn, button:has-text("Import")')).toBeVisible()
  })

  test('should display export page with all required UI elements', async ({ page }) => {
    await page.goto('http://localhost:5173/workbench/export')
    
    // Should have connection selector
    await expect(page.locator('select.io-input').first()).toBeVisible()
    
    // Should have export format buttons
    await expect(page.locator('button.format-card:has-text("Plain SQL")')).toBeVisible()
    
    // Should have export button
    await expect(page.locator('button.primary, button:has-text("Export")')).toBeVisible()
  })

  test('should switch between psql and pgdump import formats', async ({ page }) => {
    await page.goto('http://localhost:5173/workbench/import')
    
    // Default is psql
    await expect(page.locator('button.format-card:has-text("Plain SQL")')).toHaveClass(/active/)
    
    // Switch to pgdump
    await page.click('button.format-card:has-text("pg_dump archive")')
    await expect(page.locator('button.format-card:has-text("pg_dump archive")')).toHaveClass(/active/)
  })

  test('should switch between plain and archive export formats', async ({ page }) => {
    await page.goto('http://localhost:5173/workbench/export')
    
    // Default is plain
    await expect(page.locator('button.format-card:has-text("Plain SQL")')).toHaveClass(/active/)
    
    // Switch to archive
    await page.click('button.format-card:has-text("pg_dump archive")')
    await expect(page.locator('button.format-card:has-text("pg_dump archive")')).toHaveClass(/active/)
  })
})