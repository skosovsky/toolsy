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
