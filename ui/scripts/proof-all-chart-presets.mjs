import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

import { chromium } from "playwright";

function parseCookieValue(setCookieHeader = "", cookieName = "agently_session") {
  const text = String(setCookieHeader || "").trim();
  if (!text) return "";
  const target = `${cookieName}=`;
  const segments = text.split(/,(?=\s*[^;,]+=)/);
  for (const segment of segments) {
    const parts = String(segment || "")
      .split(";")
      .map((entry) => String(entry || "").trim())
      .filter(Boolean);
    const match = parts.find((entry) => entry.startsWith(target));
    if (match) {
      return match.slice(target.length).trim();
    }
  }
  return "";
}

async function parseJSONSafe(response) {
  try {
    return await response.json();
  } catch (_) {
    return null;
  }
}

async function attachSessionCookie(baseUrl, sessionId, cookieName = "agently_session") {
  const response = await fetch(`${baseUrl}/v1/api/auth/session/attach`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ sessionId }),
  });
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`Session attach failed (${response.status}): ${text}`);
  }
  const setCookie = response.headers.get("set-cookie") || "";
  const cookieValue = parseCookieValue(setCookie, cookieName);
  if (cookieValue) {
    return cookieValue;
  }
  const payload = await parseJSONSafe(response);
  if (payload?.sessionId) {
    return String(payload.sessionId);
  }
  throw new Error("Session attach succeeded but no session cookie was returned");
}

async function mintOOBSessionCookie(baseUrl, cookieName = "agently_session") {
  const secretsURL = String(process.env.OOB_SECRETS_URL || process.env.AGENTLY_OOB_SECRETS_URL || "").trim();
  if (!secretsURL) {
    throw new Error("OOB_SECRETS_URL is required");
  }
  const configURL = String(process.env.OOB_CONFIG_URL || process.env.AGENTLY_OOB_CONFIG_URL || "").trim();
  const response = await fetch(`${baseUrl}/v1/api/auth/oob`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({
      secretsURL,
      ...(configURL ? { configURL } : {}),
    }),
  });
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`OOB auth failed (${response.status}): ${text}`);
  }
  const setCookie = response.headers.get("set-cookie") || "";
  const cookieValue = parseCookieValue(setCookie, cookieName);
  if (cookieValue) {
    return cookieValue;
  }
  const payload = await parseJSONSafe(response);
  if (payload?.sessionId) {
    return attachSessionCookie(baseUrl, String(payload.sessionId), cookieName);
  }
  throw new Error("OOB auth succeeded but no session cookie was returned");
}

function extractChartSpecs(payload) {
  const visited = new Set();
  const stack = [payload];
  while (stack.length > 0) {
    const current = stack.pop();
    if (!current || typeof current !== "object") {
      continue;
    }
    if (visited.has(current)) {
      continue;
    }
    visited.add(current);
    if (Array.isArray(current.defaultChartSpecs)) {
      return current.defaultChartSpecs;
    }
    Object.values(current).forEach((value) => {
      if (value && typeof value === "object") {
        stack.push(value);
      }
    });
  }
  return [];
}

async function fetchWindowMetadata(baseBackendUrl, windowKey) {
  const response = await fetch(
    `${baseBackendUrl}/v1/api/agently/forge/window/${encodeURIComponent(windowKey)}?platform=web&formFactor=desktop&surface=browser`,
    {
      method: "GET",
      headers: { Accept: "application/json" },
    },
  );
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`Failed to fetch metadata for ${windowKey} (${response.status}): ${text}`);
  }
  const payload = await response.json();
  const chartSpecs = extractChartSpecs(payload?.data);
  return chartSpecs
    .map((entry) => ({
      title: String(entry?.title || "").trim(),
      type: String(entry?.type || "").trim().toLowerCase(),
      xField: String(entry?.xField || "").trim(),
      yFields: Array.isArray(entry?.yFields)
        ? entry.yFields.map((value) => String(value || "").trim()).filter(Boolean)
        : [],
      seriesField: String(entry?.seriesField || "").trim(),
    }))
    .filter((entry) => entry.title);
}

const FIELD_SAMPLES = {
  eventDate: ["2026-06-30", "2026-07-01", "2026-07-02"],
  channelV2: ["Display", "CTV", "Audio"],
  channelId: ["CTV", "Display", "Video"],
  channelName: ["CTV", "Display", "Video"],
  publisherId: ["Publisher A", "Publisher B", "Publisher C"],
  publisherName: ["Publisher A", "Publisher B", "Publisher C"],
  siteId: ["site-001", "site-002", "site-003"],
  siteType: ["Publisher Site", "Streaming", "Audio App"],
  mediaPlcmt: ["Banner", "CTV", "Audio"],
  pubType: ["Open Exchange", "Premium", "Streaming"],
  metrocode: ["501", "602", "803"],
  city: ["New York", "Los Angeles", "Chicago"],
  carrierId: ["Carrier A", "Carrier B", "Carrier C"],
  os: ["iOS", "Android TV", "Android"],
  brand: ["Apple", "Samsung", "Google"],
  model: ["iPhone 15", "Frame TV", "Pixel 9"],
  deviceType: ["Mobile", "CTV", "Audio"],
  agegroupId: ["25-34", "35-44", "45-54"],
  genderId: ["Female", "Male", "Unknown"],
  deviceLang: ["English", "Spanish", "French"],
  houseIncomeId: ["100K+", "150K+", "75K-100K"],
  country: ["US", "CA", "UK"],
  campaignId: ["Campaign Alpha", "Campaign Beta", "Campaign Gamma"],
};

function samplesForField(fieldName = "") {
  const normalized = String(fieldName || "").trim();
  return Array.isArray(FIELD_SAMPLES[normalized]) && FIELD_SAMPLES[normalized].length > 0
    ? FIELD_SAMPLES[normalized]
    : [`${normalized || "value"} 1`, `${normalized || "value"} 2`, `${normalized || "value"} 3`];
}

function metricValue(fieldName = "", rowIndex = 0, seriesIndex = 0) {
  const base = (rowIndex + 1) * 100;
  const seriesBump = (seriesIndex + 1) * 25;
  switch (String(fieldName || "").trim()) {
    case "avails":
      return (rowIndex + 1) * 1000000000 + seriesBump * 10000000;
    case "hhUniqs":
      return (rowIndex + 1) * 12000000 + seriesBump * 100000;
    case "minClearingPrice":
      return Number((rowIndex + 1) * 2.5 + seriesIndex).toFixed(2) * 1;
    case "totalSpend":
      return Number((rowIndex + 1) * 1750000 + seriesBump * 10000).toFixed(2) * 1;
    case "impressions":
      return (rowIndex + 1) * 125000000 + seriesBump * 1000000;
    case "clicks":
      return (rowIndex + 1) * 125000 + seriesBump * 100;
    case "ctr":
      return Number(0.05 + rowIndex * 0.01 + seriesIndex * 0.005).toFixed(3) * 1;
    default:
      return base + seriesBump;
  }
}

function fillCompanionFields(row = {}, fieldName = "", value = "") {
  const normalized = String(fieldName || "").trim();
  if (normalized === "channelId" && !row.channelName) {
    row.channelName = String(value || "");
  }
  if (normalized === "channelName" && !row.channelId) {
    row.channelId = String(value || "");
  }
  if (normalized === "publisherId" && !row.publisherName) {
    row.publisherName = String(value || "");
  }
  if (normalized === "publisherName" && !row.publisherId) {
    row.publisherId = String(value || "");
  }
}

function buildRowsForPresetRequest(preset = {}, requestPayload = {}) {
  const inputs = requestPayload?.inputs && typeof requestPayload.inputs === "object" ? requestPayload.inputs : {};
  const dimensions = Object.entries(inputs.dimensions || {})
    .filter(([, enabled]) => enabled === true)
    .map(([fieldName]) => String(fieldName || "").trim())
    .filter(Boolean);
  const measures = Object.entries(inputs.measures || {})
    .filter(([, enabled]) => enabled === true)
    .map(([fieldName]) => String(fieldName || "").trim())
    .filter(Boolean);
  const xField = String(preset?.xField || "").trim();
  const seriesField = String(preset?.seriesField || "").trim();
  const xValues = samplesForField(xField);
  const seriesValues = seriesField ? samplesForField(seriesField).slice(0, 3) : [""];
  const rows = [];
  xValues.forEach((xValue, rowIndex) => {
    seriesValues.forEach((seriesValue, seriesIndex) => {
      const row = {};
      dimensions.forEach((fieldName) => {
        if (row[fieldName] !== undefined) {
          return;
        }
        if (fieldName === xField) {
          row[fieldName] = xValue;
          fillCompanionFields(row, fieldName, xValue);
          return;
        }
        if (seriesField && fieldName === seriesField) {
          row[fieldName] = seriesValue;
          fillCompanionFields(row, fieldName, seriesValue);
          return;
        }
        const fallbackValue = samplesForField(fieldName)[0];
        row[fieldName] = fallbackValue;
        fillCompanionFields(row, fieldName, fallbackValue);
      });
      measures.forEach((fieldName) => {
        row[fieldName] = metricValue(fieldName, rowIndex, seriesIndex);
      });
      rows.push(row);
    });
  });
  return rows;
}

async function ensureDir(dir) {
  await fs.mkdir(dir, { recursive: true });
}

async function waitForChartRender(page, preset = {}) {
  await page.waitForFunction((config) => {
    const title = String(config?.title || "");
    const bodyText = document.body?.innerText || document.body?.textContent || "";
    if (!bodyText.includes(title)) {
      return false;
    }
    const root = document.querySelector(".recharts-wrapper");
    if (!root) {
      return false;
    }
    const svg = root.querySelector("svg");
    const bounds = (svg || root).getBoundingClientRect?.() || null;
    if (!bounds || Number(bounds.width || 0) < 200 || Number(bounds.height || 0) < 160) {
      return false;
    }
    const rects = Array.from(svg?.querySelectorAll("rect") || []).filter((node) => {
      const width = Number(node.getAttribute("width") || 0);
      const height = Number(node.getAttribute("height") || 0);
      return width > 1 && height > 1;
    }).length;
    const paths = Array.from(svg?.querySelectorAll("path") || []).filter((node) => {
      const box = node.getBoundingClientRect?.() || null;
      return box && (Number(box.width || 0) > 1 || Number(box.height || 0) > 1);
    }).length;
    const circles = Array.from(svg?.querySelectorAll("circle") || []).length;
    const sectors = Array.from(svg?.querySelectorAll(".recharts-pie-sector path, .recharts-sector") || []).filter((node) => {
      const box = node.getBoundingClientRect?.() || null;
      return box && Number(box.width || 0) > 1 && Number(box.height || 0) > 1;
    }).length;
    switch (String(config?.type || "").trim()) {
      case "donut":
        return sectors >= 2;
      case "horizontal_bar":
      case "bar":
        return rects >= 1 || paths >= 1;
      case "line":
        return paths >= 1 || circles >= 2;
      case "area":
        return paths >= 1;
      default:
        return rects >= 1 || paths >= 1 || circles >= 1 || sectors >= 1;
    }
  }, {
    title: String(preset?.title || ""),
    type: String(preset?.type || ""),
  }, { timeout: 120000 });
}

async function waitForRequestIncrease(requestEntries, previousCount, timeoutMs = 120000) {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    if (requestEntries.length > previousCount) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Timed out waiting for datasource request after count ${previousCount}`);
}

function requestMentionsField(payload, fieldName) {
  const needle = String(fieldName || "").trim();
  if (!needle) {
    return true;
  }
  const visited = new Set();
  const stack = [payload];
  while (stack.length > 0) {
    const current = stack.pop();
    if (current == null) {
      continue;
    }
    if (typeof current === "string" || typeof current === "number" || typeof current === "boolean") {
      if (String(current).trim() === needle) {
        return true;
      }
      continue;
    }
    if (typeof current !== "object") {
      continue;
    }
    if (visited.has(current)) {
      continue;
    }
    visited.add(current);
    if (Array.isArray(current)) {
      current.forEach((value) => stack.push(value));
      continue;
    }
    for (const [key, value] of Object.entries(current)) {
      if (String(key).trim() === needle) {
        return true;
      }
      stack.push(value);
    }
  }
  return false;
}

function validatePresetRequest(preset, requestPayload) {
  const xFieldOk = requestMentionsField(requestPayload, preset.xField);
  const yFields = Array.isArray(preset.yFields) ? preset.yFields : [];
  const missingYFields = yFields.filter((fieldName) => !requestMentionsField(requestPayload, fieldName));
  const seriesFieldOk = requestMentionsField(requestPayload, preset.seriesField);
  return {
    xFieldOk,
    seriesFieldOk,
    missingYFields,
    ok: xFieldOk && seriesFieldOk && missingYFields.length === 0,
  };
}

async function proveWindow({
  baseAppUrl,
  cookieValue,
  outputDir,
  windowKey,
  dataSourcePath,
}) {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1800 } });
  await context.addCookies([{ name: "agently_session", value: cookieValue, url: baseAppUrl }]);
  const page = await context.newPage();
  const requestEntries = [];
  let activePreset = null;
  await page.route(`**${dataSourcePath}`, async (route) => {
    const request = route.request();
    const raw = request.postData() || "";
    let payload = null;
    try {
      payload = JSON.parse(raw || "{}");
    } catch (_) {
      payload = null;
    }
    requestEntries.push({
      url: request.url(),
      method: request.method(),
      raw,
      payload,
      presetTitle: activePreset?.title || "",
    });
    const rows = buildRowsForPresetRequest(activePreset || {}, payload || {});
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        rows,
        metrics: {},
        dataInfo: { hasMore: false },
      }),
    });
  });

  await page.goto(`${baseAppUrl}/mcp-ui/forge-window?windowKey=${windowKey}`);
  await page.getByText("Run report", { exact: true }).waitFor({ timeout: 60000 });
  const beforeInitialRun = requestEntries.length;
  await page.getByRole("button", { name: "Run report", exact: true }).click();
  await waitForRequestIncrease(requestEntries, beforeInitialRun, 120000);
  await page.locator(".forge-report-builder__chart-action-button--quick").first().waitFor({ timeout: 60000 });
  const presets = await fetchWindowMetadata("http://127.0.0.1:9191", windowKey);
  const summary = [];

  for (const preset of presets) {
    const screenshotName = `${windowKey}-${preset.title.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}.png`;
    try {
      console.error(`[proof] ${windowKey} -> ${preset.title}`);
      const beforeCount = requestEntries.length;
      console.error(`[proof] opening menu ${windowKey} -> ${preset.title}`);
      await page.locator(".forge-report-builder__chart-action-button--quick").first().click();
      console.error(`[proof] waiting menu item ${windowKey} -> ${preset.title}`);
      await page.waitForFunction((title) => {
        return Array.from(document.querySelectorAll('[role="menuitem"]'))
          .some((node) => (node.innerText || node.textContent || "").includes(title));
      }, preset.title, { timeout: 60000 });
      activePreset = preset;
      console.error(`[proof] clicking menu item ${windowKey} -> ${preset.title}`);
      await page.evaluate((title) => {
        const item = Array.from(document.querySelectorAll('[role="menuitem"]'))
          .find((node) => (node.innerText || node.textContent || "").includes(title));
        if (!item) {
          throw new Error(`Preset '${title}' not found`);
        }
        item.click();
      }, preset.title);
      console.error(`[proof] waiting request ${windowKey} -> ${preset.title}`);
      await waitForRequestIncrease(requestEntries, beforeCount);
      const afterCount = requestEntries.length;
      const latestRequest = requestEntries[requestEntries.length - 1] || { payload: null, raw: "" };
      const requestValidation = validatePresetRequest(preset, latestRequest.payload);
      if (!requestValidation.ok) {
        throw new Error(
          `Preset '${preset.title}' request did not match metadata: ${JSON.stringify(requestValidation)}`,
        );
      }
      console.error(`[proof] waiting chart ${windowKey} -> ${preset.title}`);
      await waitForChartRender(page, preset);
      console.error(`[proof] capturing screenshot ${windowKey} -> ${preset.title}`);
      await page.screenshot({ path: path.resolve(outputDir, screenshotName), fullPage: true });
      summary.push({
        title: preset.title,
        status: "passed",
        requestCountDelta: afterCount - beforeCount,
        screenshot: screenshotName,
        requestValidation,
        requestPayload: latestRequest.payload,
      });
      console.error(`[proof] passed ${windowKey} -> ${preset.title}`);
    } catch (error) {
      const failureName = screenshotName.replace(/\.png$/i, "-failure.png");
      await page.screenshot({ path: path.resolve(outputDir, failureName), fullPage: true }).catch(() => {});
      summary.push({
        title: preset.title,
        status: "failed",
        screenshot: failureName,
        error: String(error?.message || error),
      });
      console.error(`[proof] failed ${windowKey} -> ${preset.title}: ${String(error?.message || error)}`);
    }
  }

  await browser.close();
  return summary;
}

async function main() {
  const baseAppUrl = "http://127.0.0.1:5173";
  const outputDir = path.resolve("test-results/report-builder-chart-presets-proof");
  await ensureDir(outputDir);
  const cookieValue = await mintOOBSessionCookie(baseAppUrl);

  const forecasting = await proveWindow({
    baseAppUrl,
    cookieValue,
    outputDir,
    windowKey: "forecastingCubeBuilder",
    dataSourcePath: "/v1/api/datasources/forecasting_cube_report/fetch",
  });

  const performance = await proveWindow({
    baseAppUrl,
    cookieValue,
    outputDir,
    windowKey: "metricReportBuilder",
    dataSourcePath: "/v1/api/datasources/metrics_ad_cube_report/fetch",
  });

  const summary = { forecasting, performance };
  await fs.writeFile(path.resolve(outputDir, "summary.json"), JSON.stringify(summary, null, 2), "utf8");
  const failures = [...forecasting, ...performance].filter((entry) => entry.status !== "passed");
  if (failures.length > 0) {
    throw new Error(`Preset proof failed for ${failures.length} preset(s). See summary.json for details.`);
  }
  console.log(JSON.stringify(summary, null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
