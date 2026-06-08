import React from "react";
import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";

import SteerQueue from "./SteerQueue.jsx";

function renderQueue(message, extras = {}) {
  return renderToStaticMarkup(
    <SteerQueue
      message={message}
      context={{
        services: {
          chat: {
            forceSteerQueuedTurn: vi.fn(),
            moveQueuedTurn: vi.fn(),
            editQueuedTurn: vi.fn(),
            cancelQueuedTurnByID: vi.fn(),
          },
        },
        ...extras,
      }}
    />,
  );
}

describe("SteerQueue", () => {
  it("renders controller-owned queued turns with AUTO badge and reason", () => {
    const html = renderQueue({
      running: true,
      queuedTurns: [
        {
          id: "turn-controller",
          preview: "Continue implementing goal summary UI",
          origin: "controller",
          statusReason: "continue active goal after successful turn",
        },
      ],
    });

    expect(html).toContain("AUTO");
    expect(html).toContain("Continue implementing goal summary UI");
    expect(html).toContain("continue active goal after successful turn");
  });

  it("omits controller chrome for normal user-queued turns", () => {
    const html = renderQueue({
      running: false,
      queuedTurns: [
        {
          id: "turn-user",
          preview: "Check the failing tests next",
          origin: "user",
        },
      ],
    });

    expect(html).toContain("Check the failing tests next");
    expect(html).not.toContain("AUTO");
  });
});
