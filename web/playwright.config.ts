import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  snapshotPathTemplate: '{testDir}/__snapshots__/{testFilePath}/{arg}{ext}',
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  use: {
    baseURL: 'http://localhost:17654',
    headless: true,
    trace: 'retain-on-failure',
  },
  expect: {
    toHaveScreenshot: {
      threshold: 0.25,
      maxDiffPixelRatio: 0.025,
    },
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
      testIgnore: /mobile-terminal\.spec\.ts/,
    },
    {
      name: 'mobile-webkit',
      testMatch: /mobile-terminal\.spec\.ts/,
      use: {
        browserName: 'webkit',
        viewport: { width: 390, height: 844 },
        hasTouch: true,
        isMobile: true,
      },
    },
  ],
})
