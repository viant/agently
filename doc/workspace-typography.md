# Workspace design-system foundation

Agently and Forge are shared open-source owners. Runtime behavior, semantic APIs,
neutral defaults and examples stay generic. Workspace-owned configuration supplies
brand colors, fonts, reviewed values, business content and variant selection.
See Forge's canonical doc/typography.md for its full foundation/component map.

The shared optional v1 contract is defined by Agently Core's
`protocol/ui/theme`. Agently validates the same role names, families and numeric
bounds. Forge owns the reusable Typography implementation and canonical neutral
fallbacks. Workspace assets own the family and reviewed values. Settings uses
`typographyProps` from `forge/components` on the actual heading, prose, label,
select and metadata elements when the active profile supplies a role weight.
Legacy profiles retain the original settings declarations. No global shell
migration or vendor-font dependency is introduced.

`npm run test:typography` retains the original family/inheritance proof and adds
`typography-components-proof.html`. That proof uses the real style manager,
WorkspaceStyleProvider, WorkspaceAppearanceSettings, Forge theme boundary,
Foundation and Typography specimens, WidgetRenderer, and Chart. By default it reads the shared
Core fixture. Optional `AGENTLY_TYPOGRAPHY_WORKSPACE_PROOF` points at bytes exported
by Core's actual workspace style service test. Optional
`AGENTLY_TYPOGRAPHY_PILOT_JSON` supplies an external workspace's container metadata,
sample values, theme, font expectation and element/action checks. Shared proof
code contains no customer-specific names or palette. It uses the real widgets,
form bindings and Toolbar. Handlers record local test events only.

The proof verifies role propagation across application/renderer boundaries at
1440px and 390px, loaded workspace family, authored identity roles, and page
containment. Screenshots are optional via `AGENTLY_TYPOGRAPHY_SCREENSHOTS` and
must stay out of product source submissions when they contain private material.
The style manager keeps its existing immutable-asset and cascade behavior.

Swift and Kotlin validators now accept and validate optional role/foundation
fields and workspace-primary/monospace names. Native rendering keeps its existing
system fallback. This is catalog compatibility, not native UI migration.

## UX1 adoption inventory

Source scan on October 22, 2026. Counts are declarations without a variable or
inherit/normal value, not a defect count; documented legacy defaults are valid.

| Stylesheet | Size | Weight | Leading | Tracking |
| --- | ---: | ---: | ---: | ---: |
| shell.css | 137 | 79 | 51 | 26 |
| rich-content.css | 14 | 7 | 6 | 4 |
| ui-settings.css | 6 | 4 | 3 | 0 |
| execution-workspace.css | 10 | 5 | 1 | 2 |

UX1 maps brand and major section headings to section; ordinary history rows,
search, composer, Send and prose to body; structural controls to label; timestamps,
status, pagination and supporting scope to metadata; compact copy to small;
technical examples to code. Chat title appearance uses heading only where it is
a page title. Markdown h1/h2 semantics remain independently correct. Selected
states should use the existing surface/color/border roles rather than adding a
new weight scale. Inline component styles and rich-content payloads also need
review in the touched scope. Do not remove unrelated legacy declarations as a
bulk cleanup or call these counts resolved by passing a font test.

The design-system workflow runs these checks. Making its status merge-required
still needs a repository administrator before the migration gate can close.


## Contributing and extending

The versioned root README and AGENTS.md point to this guide. Compose supported
components before adding a new pattern. A shared extension includes its API,
variants, keyboard/focus and data states, specimen, product consumer, migration
limit and existing contract/component checks in the same change. PR evidence
identifies those items for the existing shared-component owner's review. Never
silently enlarge a legacy allowance or move canonical values into a page.

The repository now supplies versioned entry points for Codex, Claude Code and
Cursor, linked below. The design-system workflow runs the owning contract,
component and browser checks. GitHub policy verification found no merge-required
shared checks. An administrator must require `Design system contracts` before
team-wide merge enforcement can be claimed.

## AI contributor entry points

The team uses Codex, Claude Code and Cursor. Root `AGENTS.md` links this guide;
`CLAUDE.md` imports that entry point, and `.cursor/rules/design-system.mdc` is an
always-applied thin pointer to this same guide. These files are versioned so a
fresh checkout has the usage path. Instructions guide contributors; the contract
and component checks reject defined violations. Neither replaces design review.

Mechanisms verified against [Claude Code project memory](https://code.claude.com/docs/en/memory)
and [Cursor project rules](https://cursor.com/docs/rules). The Cursor rule uses
`.mdc` frontmatter and `alwaysApply: true`; no user/team settings are changed.

The browser proof uses Playwright's matching Chromium by default. Install it with
`npx playwright install chromium` before `npm run test:typography` in ui/.
`PLAYWRIGHT_CHANNEL` can explicitly select another installed channel.
