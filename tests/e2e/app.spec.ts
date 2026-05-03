import { test, expect } from '@playwright/test'
test.describe('ChatDB End-to-End', () => {
  test('register page loads', async ({ page }) => {
    await page.goto('/register')
    await expect(page.locator('h1')).toHaveText('ChatDB')
    await expect(page.locator('input[placeholder*="production"]')).toBeVisible()
  })

  test('login page loads', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('h1')).toHaveText('ChatDB')
    await expect(page.locator('button[type="submit"]')).toHaveText('Login')
  })

  test('register then redirect to login page', async ({ page }) => {
    await page.goto('/register')
    await page.click('text=Already have an account? Sign in')
    await expect(page).toHaveURL(/\/login/)
    await expect(page.locator('h1')).toHaveText('ChatDB')
  })
})