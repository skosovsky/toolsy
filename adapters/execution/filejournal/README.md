# Local operation journal

The host selects this optional adapter for a trusted local directory on Linux or macOS. Core does not depend on filesystem or process locking. Other platforms return unsupported during construction.

Grant reservation and operation claim are one journal transaction under an interprocess advisory lock. Writes use a private temporary file, file sync, rename and directory sync. Dispatch is authorized only after the transaction is durable. Failure during persistence never returns dispatch permission; if rename happened before the failure, the recorded claim may survive and recovery must treat it as uncertain, not run the handler again.

Keep the persistent `.lock` file: deleting/replacing it while workers run breaks mutual exclusion. All writers must use this adapter. The host must secure the entire path against untrusted access, provide encryption where required, and manage quotas/retention. Network filesystems, distributed atomicity and remote exactly-once effects are not supported. The journal has a bounded total size; exhaustion fails closed. A lease expiration requires reconciliation; no automatic recovery dispatch occurs.

Open the same journal path after restart, restore host operation identities and current policies, and resume through `OperationProfile`. Grant issuance and `Resolve` remain authenticated host-only actions. Journal records contain opaque argument bindings and encoded outcomes, not raw approval UI arguments; result confidentiality is the host's responsibility.
