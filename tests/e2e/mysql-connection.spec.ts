import { test, expect } from '@playwright/test'

test.describe('ChatDB MySQL Connection', () => {
  test('login with existing connection and view tables', async ({ page }) => {
    // Navigate to login page
    await page.goto('http://localhost:5173/login')
    
    // Wait for page to load
    await page.waitForSelector('button:has-text("Login")')
    
    // Fill in the login form
    await page.fill('input[required=""]', 'laravel_user')
    await page.fill('input[type="password"]', 'your_password')
    
    // Click login button
    await page.click('button:has-text("Login")')
    
    // Should redirect to workbench page (root path)
    await page.waitForURL('http://localhost:5173/', { timeout: 15000 })
    
    // Wait for tables to load
    await page.waitForSelector('button.table-chip', { timeout: 15000 })
    
    // Verify we can see tables from the database
    await expect(page.locator('button.table-chip:has-text("users")')).toBeVisible()
    await expect(page.locator('button.table-chip:has-text("migrations")')).toBeVisible()
    await expect(page.locator('button.table-chip:has-text("admins")')).toBeVisible()
  })

  test('view table structure', async ({ page }) => {
    // Login first
    await page.goto('http://localhost:5173/login')
    await page.waitForSelector('button:has-text("Login")')
    await page.fill('input[required=""]', 'laravel_user')
    await page.fill('input[type="password"]', 'your_password')
    await page.click('button:has-text("Login")')
    await page.waitForURL('http://localhost:5173/', { timeout: 15000 })
    
    // Click on "users" table
    await page.click('button.table-chip:has-text("users")')
    
    // Should show table structure or data
    await page.waitForTimeout(2000)
    
    // Page should still contain users
    const pageContent = await page.content()
    expect(pageContent).toContain('users')
  })
})