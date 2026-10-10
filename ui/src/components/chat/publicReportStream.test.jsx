import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, it, expect, vi } from "vitest";
import { AgUiClient } from "agently-core-ui-sdk/agui";
import { AgUiViewProjection } from "agently-core-ui-sdk/aguiViewProjection";
import {
  newConversationState,
  applyEvent,
} from "agently-core-ui-sdk/chatStore/reducer";
import { projectConversation } from "agently-core-ui-sdk/chatStore/projector";
import phases from "agently-core-ui-sdk/testdata/public-highlights-report-producer.json";
vi.mock("./LazyRichContent", async () => ({
  default: (await import("./RichContent.jsx")).default,
}));
import ChatFeed from "./ChatFeedFromChatStore.jsx";
import { ConversationViewContext } from "../../context/ConversationViewContext.js";
import { DetailContext } from "../../context/DetailContext.js";
async function projected(names, trust) {
  let state = newConversationState("owned-stream");
  const projection = new AgUiViewProjection({
    profile: trust ? "agently" : "standard",
    allowHostEffects: trust,
    logicalTurnId: "owned-turn",
    conversationId: "owned-stream",
    onViewEvent: (event) => {
      state = applyEvent(state, event);
    },
    onDescriptor: () => {},
    onOutcome: () => {},
  });
  const events = names.flatMap((name) => phases[name]);
  const client = new AgUiClient({
    url: "/owned",
    threadId: "owned-stream",
    initialMessages: [],
    fetch: async () =>
      new Response(
        events
          .map((event) => "data: " + JSON.stringify(event) + "\n\n")
          .join(""),
        { headers: { "Content-Type": "text/event-stream" } },
      ),
  });
  await client.run({ runId: "owned-run" }, projection.subscriber);
  const rows = projectConversation(state);
  return rows;
}
function render(rows, developerMode) {
  return renderToStaticMarkup(
    <ConversationViewContext.Provider
      value={{ developerMode, toolFeedDock: "inline" }}
    >
      <DetailContext.Provider value={{ showDetail: () => {} }}>
        <ChatFeed conversationId="owned-stream" rowsOverride={rows} />
      </DetailContext.Provider>
    </ConversationViewContext.Provider>,
  );
}
describe("real producer public report stream in both modes", () => {
  for (const developerMode of [false, true])
    it(`retains highlights and typed progress in mode=${developerMode}`, async () => {
      const html = render(
        await projected(["highlights", "pending"], true),
        developerMode,
      );
      expect(html).toContain("Owned textual highlights");
      expect(html).toContain("Building report");
    if (developerMode) { expect(html).toContain("Preparing owned report data"); expect(html).toContain("execution-status-narration"); } else { expect(html).not.toContain("Preparing owned report data"); }
    expect(html).not.toContain("app-iteration-operational-status");
      expect(html).not.toContain("[Interactive content]");
    });
  for (const developerMode of [false, true])
    it(`keeps note and committed report in mode=${developerMode}`, async () => {
      const html = render(
        await projected(["highlights", "pending", "complete"], true),
        developerMode,
      );
      expect(html).toContain("Owned textual highlights");
      expect(html).toContain("Owned result");
    expect(html).not.toContain("execution-status-narration");
    expect(html).not.toContain("Preparing owned report data");
    expect(html).not.toContain("app-iteration-operational-status");
    });
  it("untrusted standard report does not mount native report controls", async () => {
    const html = render(
      await projected(["highlights", "pending"], false),
      false,
    );
    expect(html).not.toContain("Building report");
    expect(html).not.toContain("app-forge-fence-loading");
  });
});

