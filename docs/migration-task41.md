# Task 41 API migration

## HTTP redirects (R01)

Agents, OpenAPI and GraphQL no longer transparently reroute RPCs. Configure the
final endpoint in `agents.NewClient`, `openapi.Options.BaseURL` / source servers,
or `graphql.Introspect`. The specification URL itself must remain within its
original origin on GET redirects.

Their shared `httptool.CheckRedirectRemote` accepts only requests that started
as GET/HEAD and remain within the original scheme, hostname and effective port.
Different ports and subdomains are different origins; HTTPS downgrade and HTTP
upgrade both cross that boundary. Same-origin GET/HEAD redirects retain headers.
The default web scraper and remote document reader use the same helper, so
their GET redirects must also remain within the original origin; supply the
final URL explicitly. Their initial URL validation remains correctable; a
refused redirect does not. MCP HTTP also uses this helper: POST RPCs and DELETE
session termination do not redirect, GET/HEAD stay in the original origin, and
method-changing RPC redirects use the same typed refusal contract.

`httptool.CheckRedirectAllowed` still permits whitelisted GET/HEAD redirects,
removing Authorization, Cookie and Proxy-Authorization on origin changes.
Both helpers now refuse every redirect for an initial POST/PUT/PATCH/DELETE or
other method, even if HTTP would rewrite it to GET. GraphQL queries and
introspection use POST, so neither redirects. HTTP toolkit POST also refuses
redirects; configure its final URL in the authorized call.

A refused redirect is inspectable with `errors.As` as `*httptool.RedirectError`.
The outer `ToolError` uses nonretryable `CodeRemoteExecution`, including URL,
IP, whitelist and blacklist refusals during redirect validation. An underlying
validation cause remains inspectable, but the outer classification does not
authorize correction/retry.
The first request was already dispatched and may have produced effects; use the
host's reconciliation contract before deciding on a new request. Agent create
continues to wrap the error in its unknown-outcome envelope.

`NewSafeHTTPClient` accepts an explicit host redirect callback; hosts supplying
their own callback own its method/origin/header/body semantics. Nil disables
redirects. Consumer custom clients continue to merge timeout only; they cannot
override these adapter policies.

## Host allow/deny patterns (R02 / D17)

`MatchHost`, `HostBlocked`, `HostMatchesAllowedDomains`, `SafeDialOptions` and
toolkit domain options now share one syntax:

| Entries | Matches |
|---|---|
| `example.com` | Exact apex only |
| `.example.com` | Descendants only, including nested subdomains |
| `example.com` and `.example.com` | Apex plus all descendants |

Previously bare entries also matched descendants. Update any allowlist that
intended that behavior to include the leading-dot entry explicitly. **Update
blacklists too:** replacing an old `"evil.com"` entry with
`"evil.com", ".evil.com"` preserves the previous apex-plus-descendants denial.
This applies to `web.WithBlockedDomains` and to shared HTTP library consumers.
Leaving only the bare entry intentionally denies the exact apex alone.

Case and surrounding whitespace are ignored. A terminal DNS root dot is
normalized on both configured patterns and request hosts, so `EXAMPLE.com.` and
`example.com` have identical host policy. A leading-dot pattern does not match
the apex or a name such as `evil-example.com`. IP checks still apply at dialing.

Every matching deny entry takes precedence, including broad allow plus narrow
deny and identical suffix policies. This replaces the precomputed exact
`conflictDeny` map. Denials happen before DNS lookup or dialing. A configured
nonempty allowlist that normalizes to no usable entries remains deny-all; it
does not silently switch to permissive blacklist mode.

## Exact input numbers and structure (R03)

Validated typed, dynamic and proxy inputs now use the same lossless JSON parser
and exact-number JSON Schema compiler. Dynamic `ValidateArgs`/`Handler` maps and
numbers inside typed `any`/map/interface fields contain `json.Number`. Replace
`value.(float64)` assertions with `value.(json.Number)` and explicit `Int64` or
`Float64` conversion, handling errors. Prefer declared `int64` fields for IDs;
explicit Go float fields still choose floating-point semantics. Values already
rounded by the host before constructing a schema/input cannot be recovered.
Use `json.Number` or integral Go values for exact numeric schema constraints.

Minimum, maximum and enum validation preserves distinctions beyond 2^53,
including max int64; prepared snapshots and cache identities retain those digits.
Schema normalization preserves property names such as `id` and literal
const/enum/default objects. Internal input compilation uses the same default
JSON Schema draft 2020-12 engine as proxy/output validation; explicit supported
dialects are honored and external schema loading stays disabled.

All validated input paths reject duplicate object keys recursively (including
escaped spellings of the same key), trailing JSON documents and malformed JSON.
Root depth is zero; values deeper than 128 or documents above 100,000 value
nodes are rejected before dispatch. These are structure limits, not byte budgets;
hosts still bound input bytes and string lengths. This intentionally replaces
typed/dynamic last-key-wins behavior. Declared custom `UnmarshalJSON` methods
remain host-owned after the shared structure/schema check; they can choose their
own Go representation. Typed field-name matching follows encoding/json rules.
