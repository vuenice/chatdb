# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: import-export.spec.ts >> Import/Export functionality >> should display export page with all required UI elements
- Location: tests/e2e/import-export.spec.ts:42:3

# Error details

```
Error: expect(locator).toBeVisible() failed

Locator: locator('select.io-input').first()
Expected: visible
Timeout: 5000ms
Error: element(s) not found

Call log:
  - Expect "toBeVisible" with timeout 5000ms
  - waiting for locator('select.io-input').first()
    - waiting for" http://localhost:5173/login" navigation to finish...
    - navigated to "http://localhost:5173/login"

```

# Test source

```ts
  1  | import { test, expect } from '@playwright/test'
  2  | 
  3  | test.describe('Import/Export functionality', () => {
  4  |   // Use API to get a valid token for testing 
  5  |   test.beforeEach(async ({ page }) => {
  6  |     // First, check if we can access the workbench - if auth required, redirect to login
  7  |     await page.goto('http://localhost:5173/workbench/import')
  8  |     await page.waitForLoadState('networkidle')
  9  |     
  10 |     // Wait for either workbench or login page 
  11 |     try {
  12 |       await page.waitForURL('**/workbench**', { timeout: 5000 })
  13 |     } catch {
  14 |       // On login page - fill and submit
  15 |       const select = page.locator('select')
  16 |       if (await select.isVisible({ timeout: 3000 }).catch(() => false)) {
  17 |         await select.selectOption({ index: 0 })
  18 |         await page.fill('input[type="text"]', 'postgres')
  19 |         await page.fill('input[type="password"]', 'root')
  20 |         await page.click('button[type="submit"]')
  21 |         await page.waitForURL('**/workbench**', { timeout: 10000 }).catch(() => {})
  22 |       }
  23 |     }
  24 |   })
  25 | 
  26 |   test('should display import page with all required UI elements', async ({ page }) => {
  27 |     await page.goto('http://localhost:5173/workbench/import')
  28 |     
  29 |     // Should have connection selector (via v-model)
  30 |     await expect(page.locator('select.io-input').first()).toBeVisible()
  31 |     
  32 |     // Should have import format buttons (psql/pgdump) for PostgreSQL
  33 |     await expect(page.locator('button.format-card:has-text("Plain SQL")')).toBeVisible()
  34 |     
  35 |     // Should have file input
  36 |     await expect(page.locator('input[type="file"]')).toBeVisible()
  37 |     
  38 |     // Should have import button
  39 |     await expect(page.locator('button.import-btn, button:has-text("Import")')).toBeVisible()
  40 |   })
  41 | 
  42 |   test('should display export page with all required UI elements', async ({ page }) => {
  43 |     await page.goto('http://localhost:5173/workbench/export')
  44 |     
  45 |     // Should have connection selector
> 46 |     await expect(page.locator('select.io-input').first()).toBeVisible()
     |                                                           ^ Error: expect(locator).toBeVisible() failed
  47 |     
  48 |     // Should have export format buttons
  49 |     await expect(page.locator('button.format-card:has-text("Plain SQL")')).toBeVisible()
  50 |     
  51 |     // Should have export button
  52 |     await expect(page.locator('button.primary, button:has-text("Export")')).toBeVisible()
  53 |   })
  54 | 
  55 |   test('should switch between psql and pgdump import formats', async ({ page }) => {
  56 |     await page.goto('http://localhost:5173/workbench/import')
  57 |     
  58 |     // Default is psql
  59 |     await expect(page.locator('button.format-card:has-text("Plain SQL")')).toHaveClass(/active/)
  60 |     
  61 |     // Switch to pgdump
  62 |     await page.click('button.format-card:has-text("pg_dump archive")')
  63 |     await expect(page.locator('button.format-card:has-text("pg_dump archive")')).toHaveClass(/active/)
  64 |   })
  65 | 
  66 |   test('should switch between plain and archive export formats', async ({ page }) => {
  67 |     await page.goto('http://localhost:5173/workbench/export')
  68 |     
  69 |     // Default is plain
  70 |     await expect(page.locator('button.format-card:has-text("Plain SQL")')).toHaveClass(/active/)
  71 |     
  72 |     // Switch to archive
  73 |     await page.click('button.format-card:has-text("pg_dump archive")')
  74 |     await expect(page.locator('button.format-card:has-text("pg_dump archive")')).toHaveClass(/active/)
  75 |   })
  76 | })
```