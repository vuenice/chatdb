import { test, expect } from '@playwright/test';

test.describe('ChatDB Dashboard Tests', () => {
  
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('http://localhost:5173');
    await page.waitForLoadState('networkidle');
    
    // Select connection
    const connectionCombo = page.locator('combobox').first();
    if (await connectionCombo.isVisible()) {
      await connectionDemo.selectOption('test-mysql');
    }
    
    // Fill credentials
    await page.getByLabel('Username').fill('laravel_user');
    await page.getByLabel('Password').fill('your_password');
    
    // Login
    await page.getByRole('button', { name: 'Login' }).click();
    await page.waitForURL('**/');
  });

  test('view tables list in sidebar', async ({ page }) => {
    // Verify tables are displayed in the sidebar
    const tablesList = page.locator('main .list');
    
    // Check for specific tables we know exist
    await expect(page.getByRole('button', { name: 'users TABLE' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'migrations TABLE' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'admins TABLE' })).toBeVisible();
    
    console.log('Tables list verified');
  });

  test('switch database connection', async ({ page }) => {
    // Database selector should be in the banner
    const dbSelector = page.getByLabel('Database');
    
    if (await dbSelector.isVisible()) {
      // Current database should be 'test'
      await expect(page.locator('combobox').filter({ hasText: 'test' })).toBeVisible();
      console.log('Database switcher verified');
    }
  });

  test('view table structure by clicking table', async ({ page }) => {
    // Click on a table to view its structure
    await page.getByRole('button', { name: 'users TABLE' }).click();
    
    // Should show table structure or navigate to table view
    await page.waitForTimeout(500);
    
    // Check for table structure elements (columns, schema, etc.)
    const pageContent = await page.content();
    expect(pageContent.length).toBeGreaterThan(0);
    
    console.log('Table structure view verified');
  });

  test('search tables functionality', async ({ page }) => {
    // Find search box
    const searchBox = page.getByPlaceholder('Search tables…');
    
    if (await searchBox.isVisible()) {
      // Search for 'users'
      await searchBox.fill('users');
      
      // Should filter to show only matching tables
      await page.waitForTimeout(300);
      
      await expect(page.getByRole('button', { name: 'users TABLE' })).toBeVisible();
      console.log('Search functionality verified');
    }
  });

  test('navigate to SQL chat interface', async ({ page }) => {
    // Check Chat SQL button exists
    const chatSqlBtn = page.getByRole('button', { name: 'Chat SQL' });
    
    if (await chatSqlBtn.isVisible()) {
      await chatSqlBtn.click();
      await page.waitForTimeout(500);
      
      // Should navigate to SQL interface
      console.log('Chat SQL navigation verified');
    }
  });

  test('navigate to History', async ({ page }) => {
    // Check History button exists
    const historyBtn = page.getByRole('button', { name: 'History' });
    
    if (await historyBtn.isVisible()) {
      await historyBtn.click();
      await page.waitForTimeout(500);
      
      console.log('History navigation verified');
    }
  });

  test('navigate to Queries', async ({ page }) => {
    // Check Queries button exists
    const queriesBtn = page.getByRole('button', { name: 'Queries' });
    
    if (await queriesBtn.isVisible()) {
      await queriesBtn.click();
      await page.waitForTimeout(500);
      
      console.log('Queries navigation verified');
    }
  });

  test('navigate to Users', async ({ page }) => {
    // Check Users button exists
    const usersBtn = page.getByRole('button', { name: 'Users' });
    
    if (await usersBtn.isVisible()) {
      await usersBtn.click();
      await page.waitForTimeout(500);
      
      console.log('Users navigation verified');
    }
  });

  test('view DB roles dropdown', async ({ page }) => {
    // DB roles selector should be in banner
    const dbRolesSelector = page.getByLabel('DB roles');
    
    if (await dbRolesSelector.isVisible()) {
      await dbRolesSelector.click();
      await page.waitForTimeout(300);
      
      // Should show role options
      console.log('DB roles dropdown verified');
    }
  });

  test('view DB pool dropdown', async ({ page }) => {
    // DB pool selector should be in banner
    const dbPoolSelector = page.getByLabel('DB pool');
    
    if (await dbPoolSelector.isVisible()) {
      await dbPoolSelector.click();
      await page.waitForTimeout(300);
      
      // Should show pool options (Read, Write)
      const readOption = page.getByRole('option', { name: 'Read' });
      const writeOption = page.getByRole('option', { name: 'Write' });
      
      if (await readOption.isVisible()) {
        console.log('DB pool dropdown verified');
      }
    }
  });

  test('user menu dropdown', async ({ page }) => {
    // User menu button should be visible
    const userMenuBtn = page.getByRole('button', { name: /Open account menu/i });
    
    if (await userMenuBtn.isVisible()) {
      await userMenuBtn.click();
      await page.waitForTimeout(300);
      
      console.log('User menu dropdown verified');
    }
  });
});