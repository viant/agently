import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";

import { chromium } from "playwright";

function usage() {
  console.error("Usage: node scripts/browser-proof-runner.mjs <scenario.json> [output-dir]");
  process.exit(1);
}

function substituteEnv(value) {
  if (typeof value === "string") {
    return value.replace(/\$\{([A-Z0-9_]+)\}/g, (_, key) => {
      if (!(key in process.env)) {
        throw new Error(`Missing required environment variable: ${key}`);
      }
      return process.env[key] ?? "";
    });
  }
  if (Array.isArray(value)) {
    return value.map(substituteEnv);
  }
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key, substituteEnv(entry)]));
  }
  return value;
}

function resolveCookieValue(raw = "") {
  const text = String(raw || "").trim();
  if (!text) {
    return "";
  }
  const prefix = "agently_session=";
  if (text.startsWith(prefix)) {
    return text.slice(prefix.length).split(";")[0].trim();
  }
  return text;
}

function normalizeRouteMock(mock = {}) {
  if (!mock || typeof mock !== "object" || Array.isArray(mock)) {
    throw new Error(`Invalid route mock: expected object, got ${JSON.stringify(mock)}`);
  }
  const urlIncludes = String(mock.urlIncludes || "").trim();
  if (!urlIncludes) {
    throw new Error(`Invalid route mock: urlIncludes is required (${JSON.stringify(mock)})`);
  }
  const method = String(mock.method || "").trim().toUpperCase();
  const headers = mock.headers && typeof mock.headers === "object" && !Array.isArray(mock.headers)
    ? { ...mock.headers }
    : {};
  const status = mock.status !== undefined && mock.status !== null && mock.status !== ""
    && Number.isFinite(Number(mock.status))
    ? Number(mock.status)
    : 200;
  const times = mock.times !== undefined && mock.times !== null && mock.times !== ""
    && Number.isFinite(Number(mock.times))
    ? Math.max(0, Number(mock.times))
    : Infinity;
  const required = mock.required !== false;
  if (times === 0) {
    throw new Error(`Invalid route mock: times must be positive when specified (${JSON.stringify(mock)})`);
  }
  if (mock.json !== undefined) {
    return {
      urlIncludes,
      method,
      headers,
      status,
      times,
      required,
      hitCount: 0,
      body: JSON.stringify(mock.json),
      contentType: String(mock.contentType || "application/json").trim() || "application/json",
    };
  }
  if (mock.text !== undefined) {
    return {
      urlIncludes,
      method,
      headers,
      status,
      times,
      required,
      hitCount: 0,
      body: String(mock.text),
      contentType: String(mock.contentType || "text/plain").trim() || "text/plain",
    };
  }
  throw new Error(`Invalid route mock: either json or text is required (${JSON.stringify(mock)})`);
}

function parseCookieValue(setCookieHeader = "", cookieName = "agently_session") {
  const text = String(setCookieHeader || "").trim();
  if (!text) {
    return "";
  }
  const target = `${String(cookieName || "").trim()}=`;
  if (!target.trim()) {
    return "";
  }
  const segments = text.split(/,(?=\s*[^;,]+=)/);
  for (const segment of segments) {
    const parts = String(segment || "").split(";").map((entry) => String(entry || "").trim()).filter(Boolean);
    const match = parts.find((entry) => entry.startsWith(target));
    if (!match) {
      continue;
    }
    return match.slice(target.length).trim();
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
  const url = `${baseUrl}/v1/api/auth/session/attach`;
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ sessionId }),
  });
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    throw new Error(`Session attach failed (${response.status}): ${body}`);
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
  throw new Error("Session attach succeeded but did not return a session cookie.");
}

async function mintOOBSessionCookie(baseUrl, cookieName = "agently_session") {
  const secretsURL = String(process.env.AGENTLY_OOB_SECRETS_URL || process.env.OOB_SECRETS_URL || "").trim();
  if (!secretsURL) {
    return "";
  }
  const configURL = String(process.env.AGENTLY_OOB_CONFIG_URL || process.env.OOB_CONFIG_URL || "").trim();
  const scopes = String(process.env.AGENTLY_OOB_SCOPES || process.env.OOB_SCOPES || "")
    .split(",")
    .map((entry) => String(entry || "").trim())
    .filter(Boolean);
  const response = await fetch(`${baseUrl}/v1/api/auth/oob`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({
      secretsURL,
      ...(configURL ? { configURL } : {}),
      ...(scopes.length > 0 ? { scopes } : {}),
    }),
  });
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    throw new Error(`OOB auth failed (${response.status}): ${body}`);
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
  throw new Error("OOB auth succeeded but did not return a session cookie or session id.");
}

async function resolveSessionCookie(baseUrl, cookieName = "agently_session") {
  const directCookieValue = resolveCookieValue(process.env.AGENTLY_SESSION_COOKIE || process.env.SESSION_COOKIE || "");
  if (directCookieValue) {
    return directCookieValue;
  }
  const sessionId = String(process.env.AGENTLY_SESSION_ID || process.env.SESSION_ID || "").trim();
  if (sessionId) {
    return attachSessionCookie(baseUrl, sessionId, cookieName);
  }
  return mintOOBSessionCookie(baseUrl, cookieName);
}

function hasSessionSourceConfigured() {
  return [
    process.env.AGENTLY_SESSION_COOKIE,
    process.env.SESSION_COOKIE,
    process.env.AGENTLY_SESSION_ID,
    process.env.SESSION_ID,
    process.env.AGENTLY_OOB_SECRETS_URL,
    process.env.OOB_SECRETS_URL,
  ].some((value) => String(value || "").trim());
}

function resolveBaseUrl(scenario) {
  const baseUrl = String(process.env.BASE_URL || scenario.baseUrl || "").trim();
  if (!baseUrl) {
    throw new Error("BASE_URL or scenario.baseUrl is required.");
  }
  return baseUrl.replace(/\/+$/, "");
}

function resolveStepFile(outputDir, file) {
  return path.resolve(outputDir, String(file || "").trim());
}

async function ensureDir(dir) {
  await fs.mkdir(dir, { recursive: true });
}

async function writeFailureArtifacts(page, outputDir, error) {
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
  const screenshotPath = path.resolve(outputDir, `failure-${timestamp}.png`);
  const domPath = path.resolve(outputDir, `failure-${timestamp}.txt`);
  await ensureDir(path.dirname(screenshotPath));
  try {
    await page.screenshot({ path: screenshotPath, fullPage: true });
  } catch (_) {
    // Best-effort only.
  }
  try {
    const text = await page.evaluate(() => document.body?.innerText || document.body?.textContent || "");
    await fs.writeFile(domPath, [
      `Error: ${error?.message || error}`,
      "",
      text,
    ].join("\n"), "utf8");
  } catch (_) {
    // Best-effort only.
  }
}

async function locatorForStep(page, step) {
  switch (step.type) {
    case "clickRole":
    case "assertRoleText":
    case "waitRole":
      return page.getByRole(step.role, {
        name: step.name,
        exact: step.exact === true,
      });
    case "clickText":
    case "assertText":
    case "assertNotText":
    case "waitForText":
      return page.getByText(step.text, {
        exact: step.exact === true,
      });
    default:
      return null;
  }
}

async function pageContainsText(page, text) {
  const needle = String(text || "");
  return page.evaluate((expected) => {
    const body = document.body;
    const textContent = body?.innerText || body?.textContent || "";
    return String(textContent).includes(expected);
  }, needle);
}

async function selectorContainsText(page, selector, text, index = 0) {
  const locator = page.locator(String(selector || "")).nth(Number(index || 0));
  const count = await page.locator(String(selector || "")).count();
  if (count <= Number(index || 0)) {
    return false;
  }
  const raw = await locator.textContent();
  return String(raw || "").includes(String(text || ""));
}

async function clickSelectorContains(page, selector, text, index = 0, options = {}) {
  const targetSelector = String(selector || "").trim();
  const needle = String(text || "");
  const targetIndex = Number(index || 0);
  const locator = page.locator(targetSelector).filter({ hasText: needle });
  const target = await waitForIndexedLocator(locator, targetIndex, options.timeoutMs || 30000, true);
  await target.click({
    force: options.force === true,
  });
}

function responseEntryMatches(entry = {}, {
  urlIncludes = "",
  status = null,
  types = [],
} = {}) {
  const normalizedUrlIncludes = String(urlIncludes || "").trim();
  if (normalizedUrlIncludes && !String(entry?.url || "").includes(normalizedUrlIncludes)) {
    return false;
  }
  if (status != null && Number(entry?.status || 0) !== Number(status)) {
    return false;
  }
  const normalizedTypes = Array.isArray(types)
    ? types.map((value) => String(value || "").trim().toLowerCase()).filter(Boolean)
    : [];
  if (normalizedTypes.length > 0) {
    const responseType = String(entry?.type || "").trim().toLowerCase();
    if (!normalizedTypes.includes(responseType)) {
      return false;
    }
  }
  return true;
}

function parseResponseEntryJSON(entry = {}) {
  const text = String(entry?.text || "").trim();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

function evaluateResponseJSONExpression(entry = {}, expression = "") {
  const source = String(expression || "").trim();
  if (!source) {
    return false;
  }
  const response = parseResponseEntryJSON(entry);
  if (!response) {
    return false;
  }
  try {
    return Function("response", `return (${source});`)(response) === true;
  } catch (_) {
    return false;
  }
}

function requestEntryMatches(entry = {}, {
  urlIncludes = "",
  method = "",
} = {}) {
  const normalizedUrlIncludes = String(urlIncludes || "").trim();
  if (normalizedUrlIncludes && !String(entry?.url || "").includes(normalizedUrlIncludes)) {
    return false;
  }
  const normalizedMethod = String(method || "").trim().toUpperCase();
  if (normalizedMethod && String(entry?.method || "").trim().toUpperCase() !== normalizedMethod) {
    return false;
  }
  return true;
}

function parseRequestEntryJSON(entry = {}) {
  const text = String(entry?.postData || "").trim();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

function evaluateRequestJSONExpression(entry = {}, expression = "") {
  const source = String(expression || "").trim();
  if (!source) {
    return false;
  }
  const request = parseRequestEntryJSON(entry);
  if (!request) {
    return false;
  }
  try {
    return Function("request", `return (${source});`)(request) === true;
  } catch (_) {
    return false;
  }
}

async function scrollSelector(page, selector, { top = null, left = null, x = null, y = null, behavior = "auto", index = 0 } = {}) {
  const locator = page.locator(String(selector || ""));
  const targetIndex = Number(index || 0);
  const count = await locator.count();
  if (count <= targetIndex) {
    throw new Error(`No matching selector for scroll: ${selector} [${targetIndex}]`);
  }
  await locator.nth(targetIndex).evaluate((node, options) => {
    if (!(node instanceof HTMLElement)) {
      return;
    }
    const nextTop = Number.isFinite(Number(options?.top)) ? Number(options.top) : null;
    const nextLeft = Number.isFinite(Number(options?.left)) ? Number(options.left) : null;
    const deltaX = Number.isFinite(Number(options?.x)) ? Number(options.x) : null;
    const deltaY = Number.isFinite(Number(options?.y)) ? Number(options.y) : null;
    const nextBehavior = String(options?.behavior || "auto");
    if (nextTop !== null || nextLeft !== null) {
      node.scrollTo({
        top: nextTop !== null ? nextTop : node.scrollTop,
        left: nextLeft !== null ? nextLeft : node.scrollLeft,
        behavior: nextBehavior,
      });
      return;
    }
    node.scrollBy({
      top: deltaY !== null ? deltaY : 0,
      left: deltaX !== null ? deltaX : 0,
      behavior: nextBehavior,
    });
  }, { top, left, x, y, behavior });
}

async function evaluateChartRender(page, options = {}) {
  const selector = String(options?.selector || ".recharts-wrapper").trim() || ".recharts-wrapper";
  const titleSelector = String(options?.titleSelector || ".forge-report-builder__result-header h3").trim();
  const index = Math.max(0, Number(options?.index || 0) || 0);
  const titleIndex = Number.isFinite(Number(options?.titleIndex)) ? Math.max(0, Number(options.titleIndex)) : index;
  const titleContains = String(options?.titleContains || "").trim();
  const refreshTexts = Array.isArray(options?.refreshTexts) && options.refreshTexts.length > 0
    ? options.refreshTexts.map((entry) => String(entry || "")).filter(Boolean)
    : ["Refreshing report data", "Preparing the latest result set"];
  const errorTexts = Array.isArray(options?.errorTexts) && options.errorTexts.length > 0
    ? options.errorTexts.map((entry) => String(entry || "")).filter(Boolean)
    : ["Failed to render dashboard block"];
  const result = await page.evaluate((config) => {
    const bodyText = document.body?.innerText || document.body?.textContent || "";
    const titleNode = config.titleSelector
      ? (document.querySelectorAll(config.titleSelector)?.[config.titleIndex] || null)
      : null;
    const titleText = titleNode?.textContent || "";
    const refreshMatch = config.refreshTexts.find((entry) => entry && bodyText.includes(entry)) || "";
    const errorMatch = config.errorTexts.find((entry) => entry && bodyText.includes(entry)) || "";
    const root = document.querySelectorAll(config.selector)?.[config.index] || null;
    const svgCandidates = root
      ? (root.matches?.("svg") ? [root, ...Array.from(root.querySelectorAll?.("svg") || [])] : Array.from(root.querySelectorAll?.("svg") || []))
      : [];
    const svg = svgCandidates.reduce((best, candidate) => {
      const box = candidate?.getBoundingClientRect?.() || null;
      const area = box ? Number(box.width || 0) * Number(box.height || 0) : 0;
      const bestBox = best?.getBoundingClientRect?.() || null;
      const bestArea = bestBox ? Number(bestBox.width || 0) * Number(bestBox.height || 0) : 0;
      return area > bestArea ? candidate : best;
    }, null);
    const bounds = (svg || root)?.getBoundingClientRect?.() || null;
    const rectNodes = Array.from(svg?.querySelectorAll?.("rect") || []);
    const pathNodes = Array.from(svg?.querySelectorAll?.("path") || []);
    const lineNodes = Array.from(svg?.querySelectorAll?.("line") || []);
    const circleNodes = Array.from(svg?.querySelectorAll?.("circle") || []);
    const polygonNodes = Array.from(svg?.querySelectorAll?.("polygon") || []);
    const sectorNodes = Array.from(svg?.querySelectorAll?.(".recharts-pie-sector path, .recharts-sector") || []);
    const visibleSectors = sectorNodes.filter((node) => {
      const box = node.getBoundingClientRect?.() || null;
      if (!box) {
        return false;
      }
      const width = Number(box.width || 0);
      const height = Number(box.height || 0);
      const minSectorWidth = Number.isFinite(config.minSectorWidth) ? config.minSectorWidth : 1;
      const minSectorHeight = Number.isFinite(config.minSectorHeight) ? config.minSectorHeight : 1;
      return width >= minSectorWidth && height >= minSectorHeight;
    }).length;
    const nonZeroRects = rectNodes.filter((node) => {
      const width = Number(node.getAttribute("width") || 0);
      const height = Number(node.getAttribute("height") || 0);
      return width > 1 && height > 1;
    }).length;
    const nonEmptyPaths = pathNodes.filter((node) => String(node.getAttribute("d") || "").trim().length > 1).length;
    const visiblePaths = pathNodes.filter((node) => {
      const box = node.getBoundingClientRect?.() || null;
      if (!box) {
        return false;
      }
      const width = Number(box.width || 0);
      const height = Number(box.height || 0);
      const minPathWidth = Number.isFinite(config.minPathWidth) ? config.minPathWidth : 1;
      const minPathHeight = Number.isFinite(config.minPathHeight) ? config.minPathHeight : 1;
      return width >= minPathWidth && height >= minPathHeight;
    }).length;
    const marks = rectNodes.length + pathNodes.length + lineNodes.length + circleNodes.length + polygonNodes.length;
    const titleOk = !config.titleContains || titleText.includes(config.titleContains);
    const widthOk = !Number.isFinite(config.minWidth) || (bounds && Number(bounds.width || 0) >= config.minWidth);
    const heightOk = !Number.isFinite(config.minHeight) || (bounds && Number(bounds.height || 0) >= config.minHeight);
    const markOk = !Number.isFinite(config.minMarks) || marks >= config.minMarks;
    const rectOk = !Number.isFinite(config.minRects) || rectNodes.length >= config.minRects;
    const nonZeroRectOk = !Number.isFinite(config.minNonZeroRects) || nonZeroRects >= config.minNonZeroRects;
    const pathOk = !Number.isFinite(config.minPaths) || nonEmptyPaths >= config.minPaths;
    const visiblePathOk = !Number.isFinite(config.minVisiblePaths) || visiblePaths >= config.minVisiblePaths;
    const lineOk = !Number.isFinite(config.minLines) || lineNodes.length >= config.minLines;
    const circleOk = !Number.isFinite(config.minCircles) || circleNodes.length >= config.minCircles;
    const sectorOk = !Number.isFinite(config.minSectors) || sectorNodes.length >= config.minSectors;
    const visibleSectorOk = !Number.isFinite(config.minVisibleSectors) || visibleSectors >= config.minVisibleSectors;
    return {
      ok: titleOk
        && !refreshMatch
        && !errorMatch
        && !!(svg || root)
        && widthOk
        && heightOk
        && markOk
        && rectOk
        && nonZeroRectOk
        && pathOk
        && visiblePathOk
        && lineOk
        && circleOk
        && sectorOk
        && visibleSectorOk,
      reason: !titleOk
        ? "title"
        : refreshMatch
          ? `refresh:${refreshMatch}`
          : errorMatch
            ? `error:${errorMatch}`
            : !(svg || root)
              ? "missingChartRoot"
              : !widthOk
                ? "width"
                : !heightOk
                  ? "height"
                  : !markOk
                    ? "marks"
                    : !rectOk
                      ? "rects"
                      : !nonZeroRectOk
                        ? "nonZeroRects"
                          : !pathOk
                            ? "paths"
                          : !visiblePathOk
                            ? "visiblePaths"
                            : !lineOk
                              ? "lines"
                              : !circleOk
                                ? "circles"
                                : !sectorOk
                                  ? "sectors"
                                  : !visibleSectorOk
                                    ? "visibleSectors"
                                    : "",
      titleText,
      refreshMatch,
      errorMatch,
      bounds: bounds ? { width: Number(bounds.width || 0), height: Number(bounds.height || 0) } : null,
      counts: {
        marks,
        rects: rectNodes.length,
        nonZeroRects,
        paths: nonEmptyPaths,
        visiblePaths,
        lines: lineNodes.length,
        circles: circleNodes.length,
        polygons: polygonNodes.length,
        sectors: sectorNodes.length,
        visibleSectors,
      },
    };
  }, {
    selector,
    titleSelector,
    index,
    titleIndex,
    titleContains,
    refreshTexts,
    errorTexts,
    minWidth: Number.isFinite(Number(options?.minWidth)) ? Number(options.minWidth) : null,
    minHeight: Number.isFinite(Number(options?.minHeight)) ? Number(options.minHeight) : null,
    minMarks: Number.isFinite(Number(options?.minMarks)) ? Number(options.minMarks) : null,
    minRects: Number.isFinite(Number(options?.minRects)) ? Number(options.minRects) : null,
    minNonZeroRects: Number.isFinite(Number(options?.minNonZeroRects)) ? Number(options.minNonZeroRects) : null,
    minPaths: Number.isFinite(Number(options?.minPaths)) ? Number(options.minPaths) : null,
    minVisiblePaths: Number.isFinite(Number(options?.minVisiblePaths)) ? Number(options.minVisiblePaths) : null,
    minPathWidth: Number.isFinite(Number(options?.minPathWidth)) ? Number(options.minPathWidth) : null,
    minPathHeight: Number.isFinite(Number(options?.minPathHeight)) ? Number(options.minPathHeight) : null,
    minLines: Number.isFinite(Number(options?.minLines)) ? Number(options.minLines) : null,
    minCircles: Number.isFinite(Number(options?.minCircles)) ? Number(options.minCircles) : null,
    minSectors: Number.isFinite(Number(options?.minSectors)) ? Number(options.minSectors) : null,
    minVisibleSectors: Number.isFinite(Number(options?.minVisibleSectors)) ? Number(options.minVisibleSectors) : null,
    minSectorWidth: Number.isFinite(Number(options?.minSectorWidth)) ? Number(options.minSectorWidth) : null,
    minSectorHeight: Number.isFinite(Number(options?.minSectorHeight)) ? Number(options.minSectorHeight) : null,
  });
  return result;
}

async function waitForIndexedLocator(locator, index = 0, timeoutMs = 30000, requireVisible = false) {
  const targetIndex = Math.max(0, Number(index || 0) || 0);
  const started = Date.now();
  let lastCount = 0;
  while (Date.now() - started < timeoutMs) {
    lastCount = await locator.count();
    if (lastCount > targetIndex) {
      const target = locator.nth(targetIndex);
      if (!requireVisible || await target.isVisible()) {
        return target;
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`No matching locator at index ${targetIndex}; last count=${lastCount}`);
}

async function locatorHasVisibleMatch(locator) {
  const count = await locator.count();
  for (let index = 0; index < count; index += 1) {
    if (await locator.nth(index).isVisible()) {
      return true;
    }
  }
  return false;
}

function normalizeConsoleText(entry = {}) {
  return String(entry?.text || "").trim();
}

function consoleEntryMatches(entry = {}, pattern = "", types = []) {
  const text = normalizeConsoleText(entry);
  if (!text) {
    return false;
  }
  const normalizedTypes = Array.isArray(types)
    ? types.map((value) => String(value || "").trim().toLowerCase()).filter(Boolean)
    : [];
  const entryType = String(entry?.type || "").trim().toLowerCase();
  if (normalizedTypes.length > 0 && !normalizedTypes.includes(entryType)) {
    return false;
  }
  return text.includes(String(pattern || ""));
}

async function executeStep(page, step, outputDir, scenario = {}, runtime = {}) {
  switch (step.type) {
    case "goto": {
      const baseUrl = step.baseUrl
        ? resolveBaseUrl({ baseUrl: step.baseUrl })
        : resolveBaseUrl(scenario);
      const url = String(step.url || "").startsWith("http")
        ? String(step.url)
        : `${baseUrl}${String(step.url || "")}`;
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: step.timeoutMs || 30000 });
      return;
    }
    case "reload": {
      await page.reload({ waitUntil: "domcontentloaded", timeout: step.timeoutMs || 30000 });
      return;
    }
    case "wait": {
      await page.waitForTimeout(Number(step.ms || 500));
      return;
    }
    case "setViewport": {
      await page.setViewportSize({
        width: Number(step.width || 1280),
        height: Number(step.height || 720),
      });
      return;
    }
    case "clearLocalStorage": {
      const keys = Array.isArray(step.keys) ? step.keys.map((entry) => String(entry || "").trim()).filter(Boolean) : [];
      await page.evaluate((storageKeys) => {
        let storage = null;
        try {
          storage = window.localStorage;
        } catch (_) {
          return;
        }
        if (!storage) {
          return;
        }
        if (!Array.isArray(storageKeys) || storageKeys.length === 0) {
          storage.clear();
          return;
        }
        storageKeys.forEach((key) => storage.removeItem(key));
      }, keys);
      return;
    }
    case "eval": {
      const expression = String(step.expression || "").trim();
      if (!expression) {
        throw new Error("eval requires an expression");
      }
      await page.evaluate((source) => {
        return Function(source)();
      }, expression);
      return;
    }
    case "clearConsoleEntries": {
      runtime.consoleEntries = [];
      return;
    }
    case "clearResponseEntries": {
      const pending = Array.isArray(runtime.responseReads) ? [...runtime.responseReads] : [];
      if (pending.length > 0) {
        await Promise.allSettled(pending);
      }
      runtime.responseGeneration = Number(runtime.responseGeneration || 0) + 1;
      runtime.responseEntries = [];
      runtime.responseReads = [];
      return;
    }
    case "clearRequestEntries": {
      runtime.requestEntries = [];
      return;
    }
    case "waitForText":
    case "waitRole": {
      const locator = await locatorForStep(page, step);
      await locator.waitFor({ state: "visible", timeout: step.timeoutMs || 30000 });
      return;
    }
    case "waitForDomContains": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        if (await pageContainsText(page, step.text)) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for DOM text: ${step.text}`);
    }
    case "waitForEval": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      const expression = String(step.expression || "").trim();
      if (!expression) {
        throw new Error("waitForEval requires an expression");
      }
      while (Date.now() - started < timeoutMs) {
        const matched = await page.evaluate((source) => {
          return Boolean(Function(`return (${source});`)());
        }, expression);
        if (matched) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for expression: ${expression}`);
    }
    case "waitForSelectorContains": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        if (await selectorContainsText(page, step.selector, step.text, step.index || 0)) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for selector text: ${step.selector} -> ${step.text}`);
    }
    case "waitForResponseBodyContains": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        await Promise.allSettled(Array.isArray(runtime.responseReads) ? runtime.responseReads : []);
        const matched = (Array.isArray(runtime.responseEntries) ? runtime.responseEntries : []).some((entry) => (
          responseEntryMatches(entry, step) && String(entry?.text || "").includes(String(step.text || ""))
        ));
        if (matched) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for response body text: ${step.urlIncludes || ""} -> ${step.text}`);
    }
    case "waitForResponseJsonEval": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      const expression = String(step.expression || "").trim();
      if (!expression) {
        throw new Error("waitForResponseJsonEval requires an expression");
      }
      while (Date.now() - started < timeoutMs) {
        await Promise.allSettled(Array.isArray(runtime.responseReads) ? runtime.responseReads : []);
        const matched = (Array.isArray(runtime.responseEntries) ? runtime.responseEntries : []).some((entry) => {
          if (!responseEntryMatches(entry, step)) {
            return false;
          }
          return evaluateResponseJSONExpression(entry, expression);
        });
        if (matched) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for response JSON expression: ${expression}`);
    }
    case "waitForRequestBodyContains": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matched = (Array.isArray(runtime.requestEntries) ? runtime.requestEntries : []).some((entry) => (
          requestEntryMatches(entry, step) && String(entry?.postData || "").includes(String(step.text || ""))
        ));
        if (matched) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for request body text: ${step.urlIncludes || ""} -> ${step.text}`);
    }
    case "waitForRequestJsonEval": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      const expression = String(step.expression || "").trim();
      if (!expression) {
        throw new Error("waitForRequestJsonEval requires an expression");
      }
      while (Date.now() - started < timeoutMs) {
        const matched = (Array.isArray(runtime.requestEntries) ? runtime.requestEntries : []).some((entry) => (
          requestEntryMatches(entry, step) && evaluateRequestJSONExpression(entry, expression)
        ));
        if (matched) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for request JSON expression: ${expression}`);
    }
    case "waitForChartRender": {
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      let last = null;
      while (Date.now() - started < timeoutMs) {
        last = await evaluateChartRender(page, step);
        if (last?.ok) {
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`Timed out waiting for chart render: ${JSON.stringify(last || {})}`);
    }
    case "waitSelector": {
      const locator = page.locator(String(step.selector || ""));
      const index = Number(step.index || 0);
      const timeoutMs = Number(step.timeoutMs || 30000);
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const count = await locator.count();
        if (count > index) {
          await locator.nth(index).waitFor({ state: "visible", timeout: Math.max(250, timeoutMs - (Date.now() - started)) });
          return;
        }
        await page.waitForTimeout(250);
      }
      throw new Error(`No matching selector for step ${JSON.stringify(step)}`);
    }
    case "clickRole":
    case "clickText": {
      const locator = await locatorForStep(page, step);
      const target = await waitForIndexedLocator(locator, typeof step.index === "number" ? step.index : 0, step.timeoutMs || 30000, true);
      await target.click({
        force: step.force === true,
      });
      return;
    }
    case "clickSelector": {
      const locator = page.locator(String(step.selector || ""));
      const target = await waitForIndexedLocator(locator, Number(step.index || 0), step.timeoutMs || 30000, true);
      await target.click({
        force: step.force === true,
      });
      return;
    }
    case "fillSelector": {
      const locator = page.locator(String(step.selector || ""));
      const target = await waitForIndexedLocator(locator, Number(step.index || 0), step.timeoutMs || 30000, true);
      await target.fill(String(step.value ?? ""), {
        timeout: step.timeoutMs || 30000,
      });
      return;
    }
    case "selectSelector": {
      const locator = page.locator(String(step.selector || ""));
      const target = await waitForIndexedLocator(locator, Number(step.index || 0), step.timeoutMs || 30000, true);
      await target.selectOption(String(step.value ?? ""), {
        timeout: step.timeoutMs || 30000,
      });
      return;
    }
    case "clickSelectorContains": {
      await clickSelectorContains(page, step.selector, step.text, step.index || 0, {
        force: step.force === true,
        timeoutMs: step.timeoutMs || 30000,
      });
      return;
    }
    case "scrollSelector": {
      await scrollSelector(page, step.selector, {
        top: step.top,
        left: step.left,
        x: step.x,
        y: step.y,
        behavior: step.behavior,
        index: step.index || 0,
      });
      return;
    }
    case "assertText":
    case "assertRoleText": {
      const locator = await locatorForStep(page, step);
      if (!(await locatorHasVisibleMatch(locator))) {
        throw new Error(`Expected visible text not found: ${step.text || step.name}`);
      }
      return;
    }
    case "assertNotText": {
      const locator = await locatorForStep(page, step);
      if (await locatorHasVisibleMatch(locator)) {
        throw new Error(`Unexpected text present: ${step.text}`);
      }
      return;
    }
    case "assertDomContains": {
      if (!(await pageContainsText(page, step.text))) {
        throw new Error(`Expected DOM text not found: ${step.text}`);
      }
      return;
    }
    case "assertDomNotContains": {
      if (await pageContainsText(page, step.text)) {
        throw new Error(`Unexpected DOM text present: ${step.text}`);
      }
      return;
    }
    case "assertSelectorContains": {
      if (!(await selectorContainsText(page, step.selector, step.text, step.index || 0))) {
        throw new Error(`Expected selector text not found: ${step.selector} -> ${step.text}`);
      }
      return;
    }
    case "assertConsoleNotContains": {
      const entries = Array.isArray(runtime.consoleEntries) ? runtime.consoleEntries : [];
      const matching = entries.filter((entry) => consoleEntryMatches(entry, step.text, step.types));
      if (matching.length > 0) {
        throw new Error(`Unexpected console text present: ${step.text} :: ${JSON.stringify(matching.slice(0, 10))}`);
      }
      return;
    }
    case "assertResponseBodyContains": {
      await Promise.allSettled(Array.isArray(runtime.responseReads) ? runtime.responseReads : []);
      const entries = Array.isArray(runtime.responseEntries) ? runtime.responseEntries : [];
      const matching = entries.find((entry) => (
        responseEntryMatches(entry, step) && String(entry?.text || "").includes(String(step.text || ""))
      ));
      if (!matching) {
        throw new Error(`Expected response body text not found: ${step.urlIncludes || ""} -> ${step.text}`);
      }
      return;
    }
    case "assertResponseJsonEval": {
      const expression = String(step.expression || "").trim();
      if (!expression) {
        throw new Error("assertResponseJsonEval requires an expression");
      }
      await Promise.allSettled(Array.isArray(runtime.responseReads) ? runtime.responseReads : []);
      const entries = Array.isArray(runtime.responseEntries) ? runtime.responseEntries : [];
      const matching = entries.find((entry) => responseEntryMatches(entry, step) && evaluateResponseJSONExpression(entry, expression));
      if (!matching) {
        throw new Error(`Expected response JSON expression to match: ${expression}`);
      }
      return;
    }
    case "assertRequestBodyContains": {
      const entries = Array.isArray(runtime.requestEntries) ? runtime.requestEntries : [];
      const matching = entries.find((entry) => (
        requestEntryMatches(entry, step) && String(entry?.postData || "").includes(String(step.text || ""))
      ));
      if (!matching) {
        throw new Error(`Expected request body text not found: ${step.urlIncludes || ""} -> ${step.text}`);
      }
      return;
    }
    case "assertRequestJsonEval": {
      const expression = String(step.expression || "").trim();
      if (!expression) {
        throw new Error("assertRequestJsonEval requires an expression");
      }
      const entries = Array.isArray(runtime.requestEntries) ? runtime.requestEntries : [];
      const matching = entries.find((entry) => requestEntryMatches(entry, step) && evaluateRequestJSONExpression(entry, expression));
      if (!matching) {
        throw new Error(`Expected request JSON expression to match: ${expression}`);
      }
      return;
    }
    case "screenshot": {
      const file = resolveStepFile(outputDir, step.file);
      await ensureDir(path.dirname(file));
      await page.screenshot({
        path: file,
        fullPage: step.fullPage === true,
      });
      return;
    }
    default:
      throw new Error(`Unsupported step type: ${step.type}`);
  }
}

async function main() {
  const [scenarioPath, outputDirArg] = process.argv.slice(2);
  if (!scenarioPath) {
    usage();
  }

  const scenarioRaw = await fs.readFile(path.resolve(scenarioPath), "utf8");
  const scenario = substituteEnv(JSON.parse(scenarioRaw));
  const outputDir = path.resolve(outputDirArg || scenario.outputDir || process.cwd());

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: {
      width: Number(scenario.viewport?.width || 1280),
      height: Number(scenario.viewport?.height || 720),
    },
  });

  const baseUrl = resolveBaseUrl(scenario);
  if (scenario.requireSession === true && !hasSessionSourceConfigured()) {
    throw new Error(
      "This proof scenario requires an authenticated session, but no session source is configured. " +
      "Set AGENTLY_SESSION_COOKIE or AGENTLY_SESSION_ID, or provide AGENTLY_OOB_SECRETS_URL for OOB auth."
    );
  }
  const cookieValue = await resolveSessionCookie(baseUrl, "agently_session");
  if (cookieValue) {
    const url = new URL(baseUrl);
    await context.addCookies([
      {
        name: "agently_session",
        value: cookieValue,
        domain: url.hostname,
        path: "/",
        httpOnly: false,
        secure: false,
        sameSite: "Lax",
      },
    ]);
  }

  const page = await context.newPage();
  const routeMocks = (Array.isArray(scenario.routeMocks) ? scenario.routeMocks : [])
    .map((entry) => normalizeRouteMock(entry))
    .filter(Boolean);
  const runtime = {
    consoleEntries: [],
    requestEntries: [],
    responseEntries: [],
    responseReads: [],
    responseGeneration: 0,
  };
  if (routeMocks.length > 0) {
    await page.route("**/*", async (route) => {
      const request = route.request();
      const requestUrl = request.url();
      const requestMethod = String(request.method() || "").trim().toUpperCase();
      const mock = routeMocks.find((entry) => (
        entry.times > 0
        && requestUrl.includes(entry.urlIncludes)
        && (!entry.method || entry.method === requestMethod)
      ));
      if (!mock) {
        await route.continue();
        return;
      }
      mock.hitCount += 1;
      if (mock.times !== Infinity) {
        mock.times -= 1;
        if (mock.times === 0) {
          console.warn(`[browser-proof-runner] route mock exhausted for ${requestMethod} ${mock.urlIncludes}`);
        }
      }
      await route.fulfill({
        status: mock.status,
        headers: {
          "content-type": mock.contentType,
          ...mock.headers,
        },
        body: mock.body,
      });
    });
  }
  page.on("console", (message) => {
    runtime.consoleEntries.push({
      type: message.type(),
      text: message.text(),
    });
  });
  page.on("pageerror", (error) => {
    runtime.consoleEntries.push({
      type: "pageerror",
      text: String(error?.message || error),
    });
  });
  page.on("request", (request) => {
    runtime.requestEntries.push({
      url: request.url(),
      method: request.method(),
      postData: request.postData() || "",
    });
  });
  page.on("response", (response) => {
    const generation = Number(runtime.responseGeneration || 0);
    const read = (async () => {
      try {
        const headers = response.headers();
        const contentType = String(headers["content-type"] || headers["Content-Type"] || "").toLowerCase();
        const url = response.url();
        const shouldCaptureBody = contentType.includes("json")
          || contentType.includes("text")
          || url.includes("/v1/api/");
        const text = shouldCaptureBody
          ? await response.text().catch(() => "")
          : "";
        if (generation !== Number(runtime.responseGeneration || 0)) {
          return;
        }
        runtime.responseEntries.push({
          url,
          status: response.status(),
          type: contentType,
          text,
        });
      } catch (_) {
        if (generation !== Number(runtime.responseGeneration || 0)) {
          return;
        }
        runtime.responseEntries.push({
          url: response.url(),
          status: response.status(),
          type: "",
          text: "",
        });
      }
    })();
    runtime.responseReads.push(read);
  });
  try {
    for (const step of scenario.steps || []) {
      await executeStep(page, step, outputDir, scenario, runtime);
    }
    const missedRequiredMocks = routeMocks.filter((entry) => entry.required && entry.hitCount === 0);
    if (missedRequiredMocks.length > 0) {
      throw new Error(`Required route mocks were not hit: ${missedRequiredMocks.map((entry) => `${entry.method || "ANY"} ${entry.urlIncludes}`).join(", ")}`);
    }
  } catch (error) {
    await writeFailureArtifacts(page, outputDir, error);
    throw error;
  } finally {
    await Promise.allSettled(Array.isArray(runtime.responseReads) ? runtime.responseReads : []);
    await browser.close();
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
