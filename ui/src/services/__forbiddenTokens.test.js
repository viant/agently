/**
 * __forbiddenTokens.test.js — tripwire that prevents reintroducing legacy
 * chatState access patterns outside the already-migrated chatRuntime.js.
 *
 * After PR-0 migrates the main chat feed to chatStore, any NEW read or
 * write of `chatState.liveRows`, `chatState.transcriptRows`, or
 * `chatState.renderRows` is a regression. This test greps the tree and
 * fails if new call sites appear.
 *
 * Rationale per ui-improvement.md §7.3: no hidden side readers/writers
 * after PR-0 for the main chat path. The tripwire is mechanical — when
 * a real grep finds an unexpected occurrence, the failing test names the
 * file so the reviewer can triage.
 */

import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'fs';
import { join } from 'path';

const FORBIDDEN_PATTERNS = [
  /chatState\.liveRows/,
  /chatState\.transcriptRows/,
  /chatState\.renderRows/,
];

/**
 * Files that are *allowed* to reference the forbidden tokens. These are the
 * legacy modules themselves (still on disk while PR-0 runs dual-pipelined)
 * plus the single migrated file (chatRuntime.js) that maintains the legacy
 * writes for not-yet-migrated surfaces.
 */
const ALLOWLIST = new Set([
  'src/services/liveStreamStore.js',
  'src/services/liveStreamStore.test.js',
  'src/services/transcriptStore.js',
  'src/services/transcriptStore.test.js',
  'src/services/rowMerge.js',
  'src/services/rowMerge.test.js',
  'src/services/renderRows.js',
  'src/services/renderRows.test.js',
  'src/services/messageNormalizer.js',
  'src/services/chatRuntime.js',
  'src/services/chatRuntime.test.js',
  'src/services/chatRuntime.feeds.test.js',
  'src/services/chatService.js',
  'src/services/chatService.init.test.js',
  'src/services/chatService.submit.test.js',
  'src/services/chatService.test.js',
  // This test itself contains the forbidden patterns as string literals.
  'src/services/__forbiddenTokens.test.js',
]);

function walkJs(dir, acc, root) {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === 'dist' || entry === 'build' || entry.startsWith('.')) continue;
    const full = join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) { walkJs(full, acc, root); continue; }
    if (!/\.(js|jsx|ts|tsx|mjs)$/.test(entry)) continue;
    acc.push(full.slice(root.length + 1));
  }
}

describe('forbidden chatState.* tokens', () => {
  it('no new consumer of chatState.{liveRows,transcriptRows,renderRows} outside the allowlist', () => {
    const root = process.cwd();
    const files = [];
    walkJs(join(root, 'src'), files, root);
    const violations = [];
    for (const relative of files) {
      if (ALLOWLIST.has(relative)) continue;
      const text = readFileSync(join(root, relative), 'utf-8');
      for (const pat of FORBIDDEN_PATTERNS) {
        if (pat.test(text)) {
          violations.push(`${relative} matches ${pat}`);
          break;
        }
      }
    }
    if (violations.length > 0) {
      // eslint-disable-next-line no-console
      console.error('forbidden tokens detected:\n' + violations.join('\n'));
    }
    expect(violations).toEqual([]);
  });
});
