import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  preserveOutput: "always",
  use: {
    baseURL: "http://127.0.0.1:18080",
  },
  webServer: {
    command: "APP_PORT=18080 METRICS_PORT=19090 PPROF_PORT=16060 APP_REGION=e2e DB_PATH=:memory: go run ./cmd/server",
    url: "http://127.0.0.1:18080/health",
    timeout: 120_000,
    reuseExistingServer: !process.env.CI,
  },
});
