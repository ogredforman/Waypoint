import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { defineConfig, devices } from '@playwright/test'

// The smoke test drives the real binary (embedded SPA, mock extractor) against a throwaway
// data dir and an empty config file, so the owner's own config and database are never touched.
// Run it with `make e2e`, which builds the frontend into the embed directory first.

const port = '7429'

// The config is loaded again in each worker; the env var keeps them all on one data dir.
if (!process.env.WAYPOINT_E2E_DIR) {
  const dir = mkdtempSync(join(tmpdir(), 'waypoint-e2e-'))
  writeFileSync(join(dir, 'config.toml'), '')
  process.env.WAYPOINT_E2E_DIR = dir
}
const dataDir = process.env.WAYPOINT_E2E_DIR

export default defineConfig({
  testDir: 'e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } }],
  webServer: {
    command: 'go run ./cmd/waypoint',
    cwd: '..',
    url: `http://127.0.0.1:${port}/api/projects`,
    reuseExistingServer: false,
    timeout: 180_000,
    env: {
      WAYPOINT_PORT: port,
      WAYPOINT_DATA_DIR: dataDir,
      WAYPOINT_CONFIG: join(dataDir, 'config.toml'),
      WAYPOINT_LOG_LEVEL: 'warn',
    },
  },
})
