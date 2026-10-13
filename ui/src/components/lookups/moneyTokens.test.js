import { describe, it, expect } from 'vitest';
import { resolveMoneyPlaceholder, flattenStored, parseTokens, serializeManualToken, isValidMoney } from './tokens.js';
import { draftWithEditedChip, createEditingChipState } from './chipEditing.js';
import { resolveStarterPromptMacros } from '../chat/starterPromptMacros.js';

describe('money composer input', () => {
  it('creates inline money slots from typed and starter prompts without a registry', () => {
    const draft = resolveMoneyPlaceholder('Budget ${money} and reserve ${money}.');
    expect(parseTokens(draft).map(token => token.label)).toEqual(['$[___]', '$[___]']);
    expect(resolveStarterPromptMacros('Budget ${money}')).toBe('Budget @{money:? "$[___]"}');
    expect(resolveMoneyPlaceholder('@{order:7 "Label ${money}"}')).toBe('@{order:7 "Label ${money}"}');
  });
  it('submits the typed dollar amount even while the chip editor is still open', () => {
    const raw = '@{money:? "$[___]"}';
    const chip = createEditingChipState({ raw, name: 'money', unresolved: true });
    const draft = draftWithEditedChip(`Budget ${raw}.`, chip, '1200.50');
    expect(parseTokens(draft)[0].label).toBe('$1 200.50');
    expect(flattenStored(draft, [])).toBe('Budget $1 200.50.');
    expect(flattenStored(serializeManualToken('money', '0'), [])).toBe('$0');
  });
  it.each(['?', '-1', '1.234', 'NaN', '1e3', 'abc'])('rejects incomplete or invalid amount %s at submission', amount => {
    expect(isValidMoney(amount)).toBe(false);
    expect(() => flattenStored(`@{money:${amount} "$${amount}"}`, [], { allowUnresolvedRequired: true })).toThrow('valid money amount');
  });
  it.each(['', '10 00', 'abc'])('blocks invalid live edits %s instead of sending the previous amount', value => {
    const raw = serializeManualToken('money', '100');
    const chip = createEditingChipState({ raw, name: 'money', id: '100' });
    const draft = draftWithEditedChip(`Budget ${raw}`, chip, value);
    expect(() => flattenStored(draft, [])).toThrow('valid money amount');
  });
  it.each(['10 000', '10000', '$10 000', '10,000'])('uses default amount %s and sends only the formatted dollars', amount => {
    const resolved = resolveMoneyPlaceholder(`Budget \${money ${amount}}.`);
    expect(parseTokens(resolved)[0]).toMatchObject({ name: 'money', id: '10000', label: '$10 000' });
    expect(flattenStored(resolved, [])).toBe('Budget $10 000.');
  });
  it('edits a default grouped amount and preserves decimal precision', () => {
    const draft = resolveMoneyPlaceholder('Budget ${money 10 000}.');
    const token = parseTokens(draft)[0];
    const edited = draftWithEditedChip(draft, createEditingChipState(token), '$12 500.50');
    expect(flattenStored(edited, [])).toBe('Budget $12 500.50.');
  });
});
