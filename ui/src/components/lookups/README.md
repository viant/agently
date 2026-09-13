# Named lookups

## Request timing

`NamedLookupInput` accepts `requestTrigger: "blur"` (default) or `"change"`.
Opening a row picker fetches initial options; edits to a slash-based query are
submitted when the input loses focus by default. Change mode uses `debounceMs`.
Authored starter chips can browse a datasource directly when no dialog/window
is configured. Their picker is retained independently of slash text.

## Leading `$` skill hints

The chat composer enables skill hints with `skillsEnabled`, `skillAgentID`, and
`skillConversationID`. Typing `$` at the beginning of the prompt requests the
selected agent's available skills using the SDK `listSkills` endpoint. Name and
description are shown; typing filters the list. Arrow keys move the selection,
Enter/Tab insert `$skill-name `, and Escape closes it. Selection edits the draft;
it does not activate or execute the skill until Send. Existing prompt text and
lookup tokens after the prefix are preserved.

Hints do not open for mid-sentence dollar signs, matching Agently's leading skill
activation syntax. Changing the selected agent invalidates the previous catalog.
Loading, empty, and retry states are explicit. `/` continues to use lookup hints.
