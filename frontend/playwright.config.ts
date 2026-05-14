import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig } from '@playwright/test';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const backendDir = path.join(repoRoot, 'backend');

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30000,
  use: {
    baseURL: 'http://127.0.0.1:6366',
    headless: true,
  },
  webServer: {
    command: 'go run ./cmd/chatdb',
    cwd: backendDir,
    port: 6366,
    reuseExistingServer: !process.env.CI,
  },
});
