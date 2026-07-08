import { chromium } from "playwright";

function log(...args) {
  console.log(...args);
}

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

async function dispatchMouseEventAt(page, x, y, type) {
  await page.evaluate(({ x, y, type }) => {
    const target = document.elementFromPoint(x, y);
    const event = new MouseEvent(type, {
      bubbles: true,
      cancelable: true,
      clientX: x,
      clientY: y,
      button: 0,
    });
    (target || window).dispatchEvent(event);
  }, { x, y, type });
}

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1280, height: 960 } });
  const consoleErrors = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });
  page.on("pageerror", (err) => {
    consoleErrors.push(`pageerror: ${err.message}`);
  });

  await page.goto("http://127.0.0.1:5175/report-builder-preview.html");
  await waitForDomContains(page, "Semantic Report Builder Preview");
  await page.evaluate(() => {
    const close = Array.from(document.querySelectorAll("button")).find((entry) => (entry.innerText || entry.textContent || "").trim() === "Close");
    if (close) close.click();
  });

  log("Applying Capacity Inventory Brief starter...");
  const starterGrid = page.locator('[aria-label="Available report starters"]');
  const starterRow = starterGrid.locator(".forge-report-builder__design-source-grid-row", { hasText: "Capacity Inventory Brief" }).first();
  await starterRow.getByRole("button", { name: "Use" }).click();
  await page.waitForTimeout(500);

  await page.waitForSelector('[data-testid="report-builder-outline-node"]', { timeout: 30000 });

  const initialTitles = await getOutlineTitles(page);
  log("Initial outline order:", initialTitles);

  const dragHandles = page.locator('[data-testid="report-builder-outline-drag-handle"]');
  const dragCount = await dragHandles.count();
  log("Draggable outline handles:", dragCount);

  // Find "Top Channel KPI" node and drag handle, drag it to before "Scope".
  const kpiHandle = page.locator('[data-testid="report-builder-outline-drag-handle"][aria-label="Drag Top Channel KPI"]');
  const scopeNode = page.locator('[data-testid="report-builder-outline-node"]', { hasText: "Scope" }).first();

  const kpiBox = await kpiHandle.boundingBox();
  const scopeBox = await scopeNode.boundingBox();
  if (!kpiBox || !scopeBox) {
    throw new Error(`Could not resolve bounding boxes. kpiBox=${JSON.stringify(kpiBox)} scopeBox=${JSON.stringify(scopeBox)}`);
  }

  const startX = kpiBox.x + kpiBox.width / 2;
  const startY = kpiBox.y + kpiBox.height / 2;
  const endX = scopeBox.x + scopeBox.width / 2;
  const endY = scopeBox.y + 4; // near top of Scope node -> "before" placement

  log(`Dragging from (${startX}, ${startY}) to (${endX}, ${endY})`);

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  // Simulate mousemove in a few steps, dispatched through native mouse API so window listeners fire.
  const steps = 8;
  for (let i = 1; i <= steps; i += 1) {
    const x = startX + ((endX - startX) * i) / steps;
    const y = startY + ((endY - startY) * i) / steps;
    await page.mouse.move(x, y);
    await page.waitForTimeout(30);
  }
  await page.waitForTimeout(50);

  const midDragTitles = await getOutlineTitles(page);
  const dropTargetClass = await page.evaluate(() => {
    const node = document.querySelector('[data-testid="report-builder-outline-node"].is-drop-before, [data-testid="report-builder-outline-node"].is-drop-after');
    return node ? node.className : null;
  });
  log("Mid-drag outline order (should be unchanged until drop):", midDragTitles);
  log("Drop target indicator class during drag:", dropTargetClass);

  await page.mouse.up();
  await page.waitForTimeout(100);

  const finalTitles = await getOutlineTitles(page);
  log("Final outline order after drop:", finalTitles);

  const expected = ["Top Channel KPI", "Scope", "Active Drill Path", "Inventory Outlook"];
  const matches = JSON.stringify(finalTitles) === JSON.stringify(expected);
  log("Reorder matched expectation:", matches, "expected:", expected);

  if (consoleErrors.length > 0) {
    log("Console/page errors observed:");
    consoleErrors.forEach((e) => log(" -", e));
  } else {
    log("No console/page errors observed.");
  }

  await browser.close();

  if (!matches) {
    process.exitCode = 1;
  }
  if (consoleErrors.length > 0) {
    process.exitCode = 1;
  }
}

main().catch((err) => {
  console.error("PROOF_SCRIPT_FAILED", err);
  process.exitCode = 1;
});
