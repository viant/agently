import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";

const scenarioPath = String(process.env.FORECASTING_LIVE_SCENARIO || "").trim();

if (!scenarioPath) {
  console.error("FORECASTING_LIVE_SCENARIO is required. Point it at the steward forecasting proof scenario JSON.");
  process.exit(1);
}

const outputDir = process.env.FORECASTING_LIVE_OUTPUT_DIR
  ? path.resolve(process.env.FORECASTING_LIVE_OUTPUT_DIR)
  : path.resolve("test-results/forecasting-proof-harness-live");

const runnerPath = path.resolve("scripts/browser-proof-runner.mjs");
const result = spawnSync(
  process.execPath,
  [runnerPath, path.resolve(scenarioPath), outputDir],
  {
    stdio: "inherit",
    env: process.env,
  },
);

process.exit(result.status ?? 1);
