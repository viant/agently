import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, it, expect } from "vitest";
import RichContent from "./RichContent.jsx";
import { ConversationViewContext } from "../../context/ConversationViewContext.js";
describe("typed report progress without author JSON", () => {
  for (const developerMode of [false, true])
    it(`renders typed progress in developerMode=${developerMode}`, () => {
      const html = renderToStaticMarkup(
        <ConversationViewContext.Provider value={{ developerMode }}>
          <RichContent
            content="[Interactive content]"
            renderedContent={{
              schemaVersion: "1",
              parts: [{ kind: "text", text: "[Interactive content]" }],
              reports: [{ scope: "message", id: "owned", status: "rendering" }],
            }}
          />
        </ConversationViewContext.Provider>,
      );
      expect(html).toContain("Building report");
      expect(html).not.toContain("[Interactive content]");
    });
  it("does not suppress unrecognized or incomplete fallback", () => {
    const html = renderToStaticMarkup(
      <RichContent
        content="[Interactive content]"
        renderedContent={{
          schemaVersion: "1",
          parts: [{ kind: "text", text: "[Interactive content]" }],
          reports: [{ scope: "message", id: "owned", status: "incomplete" }],
        }}
      />,
    );
    expect(html).toContain("[Interactive content]");
  });
});
