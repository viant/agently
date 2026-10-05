#!/usr/bin/env bash
# CLI-first actual-browser proof. Start the owned assembly/mock and harness first.
set -euo pipefail
command -v npx >/dev/null
agui_fixture_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$agui_fixture_root/../agently-core-ag-ui/examples/ag-ui-shell"
agui_pwcli="${AGUI_PWCLI:-/Users/awitas/.codex/skills/playwright/scripts/playwright_cli.sh}"
agui_browser_session="${AGUI_BROWSER_SESSION:-agui-assembly-proof}"
agui_browser_url="${AGUI_BROWSER_URL:-http://127.0.0.1:5193/}"
mkdir -p output/playwright
"$agui_pwcli" -s="$agui_browser_session" open "$agui_browser_url" --headed
trap '"$agui_pwcli" -s="$agui_browser_session" close >/dev/null' EXIT
"$agui_pwcli" -s="$agui_browser_session" snapshot
"$agui_pwcli" -s="$agui_browser_session" run-code 'async page => {
 const streams=[]; const requests=[];
 page.on("response",r=>{if(r.url().endsWith("/v1/ag-ui/run")&&r.request().method()==="POST")streams.push(r.text());});
 page.on("request",r=>{if(r.url().endsWith("/v1/ag-ui/run")&&r.method()==="POST")requests.push(r.postDataJSON());});
 const chat=page.getByRole("region",{name:"Agent chat"});
 await page.getByTestId("copilot-chat-textarea").fill("Hello browser fixture");
 await page.getByTestId("copilot-chat-textarea").press("Enter");
 await chat.getByText("Hello from the local Agently AG-UI assembly fixture.",{exact:true}).waitFor();
 await page.screenshot({path:"output/playwright/browser-chat.png"});
 await page.getByRole("textbox",{name:"Agent ID",exact:true}).fill("tool_fixture");
 await page.getByRole("textbox",{name:"Model",exact:true}).fill("local_mock");
 await page.getByTestId("copilot-chat-textarea").fill("fixture-tool please");
 await page.getByTestId("copilot-chat-textarea").press("Enter");
 await page.getByRole("button",{name:"system/os/getEnv Done",exact:true}).click();
 await chat.getByText("{\"values\":{\"AGENTLY_AGUI_FIXTURE_VALUE\":\"fixture-value\"}}",{exact:true}).waitFor();
 await page.screenshot({path:"output/playwright/browser-tool.png"});
 await page.getByRole("button",{name:"Discover capabilities",exact:true}).click();
 await page.getByRole("status").getByText("Capabilities received.",{exact:true}).waitFor();
 await page.getByText("Agently capabilities",{exact:true}).click();
 await page.getByRole("textbox",{name:"Agent ID",exact:true}).fill("simple");
 await page.getByTestId("copilot-chat-textarea").fill("fixture-forge please");
 await page.getByTestId("copilot-chat-textarea").press("Enter");
 await chat.getByText("Done.",{exact:true}).waitFor();
 const text=await chat.innerText();
 if(!text.includes("Report:")||!text.includes("[Interactive content]"))throw new Error("Missing safe Forge fallback");
 if(text.includes("private_authoring_key")||text.includes("forge-ui")||text.includes("forge-data"))throw new Error("Authoring payload leaked into chat");
 const events=(await Promise.all(streams)).flatMap(wire=>wire.split(/\r?\n\r?\n/).flatMap(frame=>frame.split(/\r?\n/).filter(line=>line.startsWith("data:")).map(line=>JSON.parse(line.slice(5)))));
 if(!events.some(e=>e.type==="ACTIVITY_SNAPSHOT"&&e.activityType==="agently.rendered-content"&&e.content.version==="1"&&e.content.renderedContent))throw new Error("Browser did not receive typed Forge activity");
 if(!requests.some(r=>r.forwardedProps?.agently?.payload?.agentId==="tool_fixture"&&r.forwardedProps.agently.payload.model==="local_mock"))throw new Error("Extension selection not sent");
 await page.screenshot({path:"output/playwright/browser-forge.png"});
 console.log("Actual CopilotKit browser chat, selected backend tool/result, capabilities, and safe Forge fallback passed");
}'
"$agui_pwcli" -s="$agui_browser_session" console error
