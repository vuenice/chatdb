import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30000,
  use: {
    baseURL: 'http://localhost:3000',
    headless: true,
  },
  webServer: {
    command: 'cd /home/kirti/code/chatdb/backend && go run ./cmd/chatdb -config chatdb.config.json',
    port: 3000,
    reuseExistingServer: !process.env.CI,
  },
});