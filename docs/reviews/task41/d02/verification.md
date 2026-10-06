# D02 neutral control verification

UIActionSignal/ErrUIAction removed; HostEventSignal/ErrHostEvent is named bounded
JSON data with no core dispatcher/authority. Pause/Yield/Halt describe host-owned
continuation requests, not registry/global cancellation or durable scheduling.
Terminal Controls are declarations; CompletionPolicy remains routing metadata.

Signals/list share inclusive64KiB field bytes, count64, eventname128byte ASCII
rules and UTF-8/strictJSON checks. Typed-nil/unsupported/malformed controls fail
INTERNAL control_contract before delivery/error normalization. Preparation copies
built-in structs and host payload bytes. JSONResultCodec checks on encode/decode,
uses host_event kind and rejects legacyui; no automatic authority-bearing migration.

Permanent AAA regressions cover individual/list limits, invalidJSON/UTF8/depth/
duplicatekeys, producer snapshot, delivery failure/nil callback, public typed
results, codec roundtrip/legacy rejection, middleware/control and independent
subsequent call. Parent controlsuite racecount3PASS1.660s; rootfullracePASS
(generator26.795s), humanracecount3PASS1.718s, OTelracePASS1.487s;
allthree pinnedmodulelints0. Runnable host allowlist example exits0.

Limits: delivery bounds do not prevent producer allocation or preempt an ordinary
producer ignoring callback errors; consumers must not race mutations during
capture/encoding. Host owns action schema, authorization, UI adapter, continuation
persistence and scheduling. No live shell/UI integration or general scheduler
acceptance claimed. Both independent acceptances100% (five20% criteria), no unresolved detected defects.
Reviewer logs and adversarial fixture sources are retained alongside reports.
