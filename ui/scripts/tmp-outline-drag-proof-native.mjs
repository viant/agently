import { chromium } from "playwright";

async function waitForDomContains(page, text, timeoutMs = 30000) {
  await page.waitForFunction(
    (needle) => (document.body.innerText || "").includes(needle),
    text,
    { timeout: timeoutMs },
  );
}

async function getOutlineTitles(page) {
  return page.evaluate(() => Array.from(
    document.querySelectorAll('[data-testid="report-builder-outline-node"] strong'),
  ).map((node) => (node.textContent || "").trim()));
}

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1280, height: 960 } });
  page.on("pageerror", (err) => console.log("pageerror:", err.message));

  await page.goto("http://127.0.0.1:5175/report-builder-preview.html");
  await waitForDomContains(page, "Semantic Report Builder Preview");
  await page.evaluate(() => {
    const close = Array.from(document.querySelectorAll("button")).find((entry) => (entry.innerText || entry.textContent || "").trim() === "Close");
    if (close) close.click();
  });

  const starterGrid = page.locator('[aria-label="Available report starters"]');
  const starterRow = starterGrid.locator(".forge-report-builder__design-source-grid-row", { hasText: "Capacity Inventory Brief" }).first();
  await starterRow.getByRole("button", { name: "Use" }).click();
  await page.waitForSelector('[data-testid="report-builder-outline-node"]', { timeout: 30000 });

  console.log("Initial outline order:", await getOutlineTitles(page));

  // Dispatch a real HTML5 DnD sequence using a shared DataTransfer, bypassing OS-level drag detection.
  const result = await page.evaluate(() => {
    function fire(target, type, dataTransfer, clientY) {
      const event = new DragEvent(type, {
        bubbles: true,
        cancelable: true,
        composed: true,
        clientY: clientY || 0,
      });
      Object.defineProperty(event, "dataTransfer", { value: dataTransfer });
      target.dispatchEvent(event);
      return event;
    }
    const handle = document.querySelector('[data-testid="report-builder-outline-drag-handle"][aria-label="Drag Top Channel KPI"]');
    const scopeNode = Array.from(document.querySelectorAll('[data-testid="report-builder-outline-node"]'))
      .find((n) => (n.textContent || "").includes("Scope"));
    if (!handle || !scopeNode) {
      return { error: "missing handle or target", hasHandle: !!handle, hasScope: !!scopeNode };
    }
    const dt = new DataTransfer();
    fire(handle, "dragstart", dt);
    const rect = scopeNode.getBoundingClientRect();
    fire(scopeNode, "dragover", dt, rect.top + 2);
    fire(scopeNode, "drop", dt, rect.top + 2);
    fire(handle, "dragend", dt);
    return { error: null };
  });
  console.log("Native DnD dispatch result:", result);

  await page.waitForTimeout(100);
  console.log("Final outline order:", await getOutlineTitles(page));

  await browser.close();
}

main().catch((err) => {
  console.error("PROOF_SCRIPT_FAILED", err);
  process.exitCode = 1;
});
