# MCP skills bridge

Updated 2026-09-21. This supersedes the earlier resource-list-only proposal.

## Official contract

The source of truth is the stable SEP-2640 / `io.modelcontextprotocol/skills`
extension against MCP 2026-07-28:
[stable specification](https://github.com/modelcontextprotocol/ext-skills/blob/0e85d4db8860a305c857f26fdede64f416675b92/specification/stable/skills.mdx).

Servers declare the extension through `server/discover`. Native
`skills/list` and `skills/get` return complete frontmatter and file manifests.
Bytes travel through `resources/read`. Optional `resources/directory/read`
is not implemented here. A resource URI scheme alone never establishes that
a resource is a skill. Upstream schemes other than `skill` are supported.

## One global Agently service

The model sees only:

```text
llm/skills:list
llm/skills:get
llm/skills:activate
```

There are no per-server Agently skill tools. The bridge in `service/skill`
uses the existing MCP manager, whose managed client forwards native
`ListSkills` and `GetSkill` operations with the caller's context.

```text
global list
  -> local metadata
  -> enabled MCP sources under the current user
  -> discover extension capability
  -> skills/list with bounded pagination
  -> merge source-qualified references

global get / activate
  -> resolve reference and agent visibility
  -> skills/get for the exact upstream URI
  -> validate manifest
  -> resources/read
  -> verify size, digest, and complete frontmatter
  -> return inspected content or activate through Agently
```

MCP skill discovery runs on demand. Startup, ordinary prompt binding, and
local filesystem watching do not enumerate MCP skill catalogs.

## Configuration

Extend an existing MCP client definition:

```yaml
name: github
transport:
  type: streamable
  url: https://example.com/mcp
# Retain the deployment's existing structured auth configuration.
skillDiscovery:
  enabled: true
  catalogId: github
  maxPages: 20
  maxScannedItems: 5000
  maxSkills: 500
  maxMetadataBytes: 1048576
  maxSkillBytes: 16777216
  timeoutSec: 10
```

Discovery defaults to disabled. The catalog ID defaults to the configured
server name; provide a unique URI-safe ID when necessary. Agent positive and
negative skill patterns apply to list and get. Empty agent skill selection
remains closed. There is no persistent remote catalog cache.

## Identity and direct lookup

Names are labels, not identifiers. Two skills on one server can share a name.
The bridge preserves the pair of configured server identity and exact upstream
URI. Results contain portable `name`, readable `qualifiedName`, and a durable
host reference:

```text
skill://github/review~<base64url-upstream-uri>~<manifest-sha256>
```

This distinguishes same-name entries and binds subsequent reads to the manifest
revision. A readable `skill://github/review` or `github/review` is accepted only
when it resolves unambiguously. Local bare-name precedence remains unchanged.

Global get/activate accept a legacy `name`, a host `uri`, or an explicit
configured `server` plus upstream `uri`:

```json
{"server":"github","uri":"docs://team/review/SKILL.md"}
```

Direct references are confirmed by native `skills/get` and work even when the
skill is omitted from a partial listing. The URI authority never chooses a
network endpoint. References grant no permission and are always checked under
the current principal.

## Legacy tools/call adapter

Older servers can opt into an explicit mapping:

```yaml
skillDiscovery:
  enabled: true
  tools:
    list: skills_list
    get: skills_get
```

Native SEP-2640 takes precedence when advertised. Otherwise the adapter calls
those configured tools with `{cursor}` or `{uri}`. Results must be structured
JSON or one JSON text item, with `{"skills":[...]}` and `{"skill":...}` envelopes
containing SEP-2640-shaped entries. The adapter supplies private, zero-TTL
envelope metadata; it does not advertise extension support for the server.

Loading requires complete static manifests. Dynamic entries can be listed but
are declined on load. Body-only legacy results need a separate explicit format
adapter; the bridge never fabricates integrity guarantees. File bytes still
use `resources/read`.

## Principal and permission boundaries

Remote discovery requires an effective authenticated user. Conversation IDs
alone are insufficient. Managed connections include the effective user when no
custom identity extractor is installed.

Protected MCP tool definitions are no longer added to the shared registry.
External tool lookups use request context; loopback addresses and previous
cache hits do not authorize background warming. This version conservatively
performs all external discovery on demand. Optional global caching of explicitly
public, invariant lists remains an optimization requiring a credential-free
discovery client separate from protected reads and calls.

Remote `allowed-tools` entries are qualified into `requestedTools` for
inspection. They do not grant executable permissions: SEP-2640 requires
explicit, content-bound user approval before widening the tool surface.
Executable `allowed-tools` is therefore cleared for remote skills.

Remote preprocessing is rejected. Runtime constraints prevent host execution
and cross-origin calls while acting on a remote skill. The source is tagged
when skill content enters model context. Existing tool-surface and execution
policy checks continue to apply. The original server document is never
rewritten or stored as a trusted local skill.

## Verification and supporting files

Every skill entry is validated using the MCP protocol library. Each document
must have the exact URI, supported MIME type, valid UTF-8, matching raw-byte
size, matching SHA-256 digest, and frontmatter equal to the held entry.
The default per-skill capacity covers 512 files and 16 MiB.

The host reference includes a digest of the full entry. A changed manifest
requires listing/selecting the updated reference; an existing active reference
cannot silently adopt new files or bytes.

Global `get` is side-effect-free. It does not activate, preprocess, start
children, emit activation events, or execute tools. Add `path` to read a
supporting file under the same skill root and originating server; the file must
be in the verified manifest. Nested SKILL.md files read this way remain data.

Qualified identities flow through activation, history, runtime state, and
child contexts. Subsequent requests revalidate access under current credentials.

## Outward MCP exposure

The MCP expose handler supports native skill methods through an optional
SkillProvider. Explicit `skillItems` patterns select local trees for export
under `skill://agently/<name>/SKILL.md`. The protocol compiler produces sealed
manifests, while resource reads preserve existing workspace UI behavior.
Listing uses revision-bound cursors and private, zero-TTL responses.
Resource-only servers can configure skillItems without tool patterns.

Remote federation is not recursively re-exported, preventing aggregation cycles.
Optional directory browsing is not advertised.

## Dependencies and tests

Current upstream commits are pinned in go.mod. Local replacements are enabled
while dependency corrections remain unpublished:

```go
replace github.com/viant/mcp => ../mcp
replace github.com/viant/mcp-protocol => ../mcp-protocol
```

The local protocol schema includes required cache fields on GetSkillResult;
the MCP server emits them. Frontmatter parsing rejects non-string YAML mapping
keys before coercion.

Tests cover principal isolation, no body reads during listing, native JSON-RPC
dispatch, same-name skills at distinct paths, alternate URI schemes, direct
get without enumeration, manifest changes, malformed content, bounded paging,
supporting-file confinement, permission non-escalation, and the configured
tool-call adapter. Focused race and local-skill regression tests accompany the
implementation.
