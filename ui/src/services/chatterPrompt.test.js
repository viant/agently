import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const repoRoot = '/Users/awitas/go/src/github.com/viant/agently';
const chatterUserPromptPath = path.join(repoRoot, 'bootstrap/defaults/agents/chatter/prompt/user.tmpl');
const chatterSystemPromptPath = path.join(repoRoot, 'bootstrap/defaults/agents/chatter/prompt/system.tmpl');

describe('chatter prompt templates', () => {
  it('injects structured JSON context into the user prompt', () => {
    const source = fs.readFileSync(chatterUserPromptPath, 'utf8');

    expect(source).toContain('{{ .ContextJSON }}');
    expect(source).not.toContain('{{.Context}}');
  });

  it('treats client runtime context as authoritative and not user-elicitable', () => {
    const source = fs.readFileSync(chatterSystemPromptPath, 'utf8');

    expect(source).toContain('Never elicit values already present in `Context.client`.');
    expect(source).toContain('do not ask the user for `platform`, `kind`, `surface`, `formFactor`, or client `capabilities`');
    expect(source).toContain('Do not turn transport or rendering metadata into user questions.');
    expect(source).toContain('Do not guess when a concrete task request is missing critical information.');
    expect(source).toContain('A greeting, pleasantry, or simple conversational opener is already a complete input.');
    expect(source).toContain('If the user says `hi`, `hello`, `hey`, `thanks`, or similar, respond conversationally in one short message and do not use elicitation or tools.');
    expect(source).toContain('For lightweight conversation such as greetings, pleasantries, or simple everyday questions, answer directly and do not elicit extra fields.');
    expect(source).toContain('Do not ask for output format, desired output, target platform, or documentation URLs unless the user explicitly asked for a deliverable that truly depends on them.');
  });
});
