# R08 — coherent concurrent session configuration

Scope: row 08. No production publication/push.

## Contract and implementation

Session configuration is one immutable registry/binding pair published through
atomic.Pointer. Rebind constructs the target outside locks, validates against the
current configuration, then CAS publishes; a failed CAS repeats validation against
the new current pair. Execute/RunCall capture one configuration per invocation.
RunCall manifest/completion policy and dispatch use that same registry even when
a manifest callback rebinds the session. Public Binding clones all retained slices.

ExportSnapshot captures the binding and clones state-map slots before encoding.
Codecs and MarshalJSON run without state/configuration locks. ExportCheckpoint
copies the inner snapshot binding to its outer metadata; no second current-config
load can mix them. Import callbacks execute before atomic state-map replacement.
Incompatible Rebind/import leaves prior configuration/state intact on rejection.
Referenced host values require immutability or codec-owned synchronization; map
cloning does not deep-copy BYOT data or create a transaction across external effects.
Registries and codec registrations remain stable after setup. R09 independently
owns late codec registration/coherent state-schema lifecycle remediation.

## Regression and checks

- [Previous commit baseline](r08/baseline.log), [probe source](r08/baseline-probe.go.txt):
  behavioral FAIL on d1102f6, repeated data races, wrong registry during RunCall,
  and codec plus MarshalJSON callback lock blockage.
- [Root race](r08/root-race.log): PASS, exit 0, including release56.900s and
  generator36.506s. [Targeted race](r08/targeted-race.log): count5 PASS.
- [Root lint](r08/lint.log): zero issues; git diff --check passes.
- AAA fixtures cover concurrent Rebind/Execute/RunCall/Binding/checkpoint/import,
  detached bindings, semantic RunCall capture, codec/Marshal reentry and unchanged
  state/configuration after incompatible Rebind. Test helper extraction corrected
  initial cognitive complexity; benchmark wrapping corrected a golines gate.

## Independent final acceptance

r08_acceptance_a and r08_acceptance_b: **100%, accepted**, five criteria20/20 each,
no unresolved detected defects. A independently repeats targeted race12 times,
then an external overlay probe25 times: in-flight calls retain their original
handler; MarshalJSON rebind/state mutation completes without locking and newly
added state slots do not enter the already captured snapshot. B independently
runs16 workers ×250 operations ×3 repetitions plus codec reentry, failed decode,
binding isolation and nil cases. Both inspect the final root race/baseline logs
and independently repeat the final lint after benchmark formatting.

Evidence: [A race](r08/review-a-race.log), [A probes](r08/review-a-probe-final.log),
[A source](r08/review-a-probe.go.txt), [A final lint](r08/review-a-lint-final.log),
[B verdict](r08/review-b-verdict.md), [B probes](r08/review-b-probe.log),
[B source](r08/review-b-probe.go.txt), [B lint](r08/review-b-lint.log).
B's external disposable harness disables sumdb only for that probe because the
sandbox prevented a host sumdb latest-file write; normal project checks preserve
checksum policy. Its module metadata is retained with the probe.

## Benchmarks and limits

[Previous commit](r08/benchmark-baseline.log) and
[current](r08/benchmark-current.log) run sequentially, three200ms samples each
on macOS/M1 Max. Public concurrent Execute retains200allocs/op. A32-slot snapshot
changes104→108allocs/op and approximately4349→6744bytes/op because it clones the
map before host callbacks. Snapshot times are about9.5–10.0µs before and10.4µs
after in these short samples; no statistical performance guarantee is claimed.
The allocation cost buys callback reentry without holding a state lock.

These tests are local macOS runs; Linux live execution is not claimed. Snapshot
captures state slots and a binding, not a transaction over caller-mutated objects.
Percentages measure the explicit five criteria, not universal bug freedom.
