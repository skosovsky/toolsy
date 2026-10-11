# SDK-only characterization fixture

Run GOWORK=off go test -race -count=1 ./... here. This module imports only the
published SDK v1.8.0; it has no toolsy or SDK replace directives.

Assertions intentionally describe known SDK limitations, not the desired upstream
contract. See ../../README.md for numeric/raw/unknown-field/cache/lifecycle
constraints and issues. ProgressAfterSDKTerminalBeforeMiddlewareReturns is a
public-middleware reproduction of the missing pre-Await retirement boundary.
Subscription ACK/filter/error handles and cancellable Wait are absent from the
public SDK API; inspect the pinned source/client.go on upgrade. Do not add a
private API access or transport wrapper to turn that absence into fake parity.

Before upgrading, review assertion changes and upstream issues, then remove fixed
limitations from the module contract and run the adapter/core/consumer suites.
