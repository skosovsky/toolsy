# Local operation journal

The host selects this optional adapter for a trusted local directory on Linux or macOS. Core does not depend on filesystem or process locking. Other platforms return unsupported during construction.

Grant reservation and operation claim are one journal transaction under an interprocess advisory lock. Writes use a private temporary file, file sync, rename and directory sync. Dispatch is authorized only after the transaction is durable. Failure during persistence never returns dispatch permission; if rename happened before the failure, the recorded claim may survive and recovery must treat it as uncertain, not run the handler again.

Keep the persistent `.lock` file: deleting/replacing it while workers run breaks mutual exclusion. All writers must use this adapter. The host must secure the entire path against untrusted access, provide encryption where required, and manage quotas/retention. Network filesystems, distributed atomicity and remote exactly-once effects are not supported. The journal has a bounded total size; exhaustion fails closed. A lease expiration requires reconciliation; no automatic recovery dispatch occurs.

Open the same journal path after restart, restore host operation identities and current policies, and resume through `OperationProfile`. Grant issuance and `Resolve` remain authenticated host-only actions. Journal records contain opaque argument bindings and encoded outcomes, not raw approval UI arguments; result confidentiality is the host's responsibility.

## Complexity and quota

This is a bounded local reference adapter. Every `Inspect` reads, decodes, copies
and validates the complete image under the journal lock. Every successful mutation
also snapshots/encodes the full image, writes a temporary file and syncs file and
directory. CPU/memory scale with total image size and record/attempt count; disk I/O
is O(total encoded bytes), not O(one operation). Concurrent handles/processes
serialize behind the persistent lock. No incremental index, pruning or background
maintenance is provided. It is unsuitable for an indefinitely growing history.

`Open(path, 0)` selects an 8MiB encoded-image cap; positive caps are inclusive.
Exceeding the cap fails closed before dispatch permission. Temporary serialization
and decoded maps require additional memory beyond that file cap. Quota failure
never permits clearing history or treating an old intent as new. Unknown outcomes
and consumed approvals continue to require authenticated host reconciliation.

## Host maintenance

Monitor file size and transaction latency, leave storage headroom for replacement
and backups, and provision a larger finite cap before exhaustion when appropriate.
For backup/restore/migration, quiesce **all** writers and readers, keep the `.lock`
file/inode stable, secure backups like the original results, and validate the full
image before reopening. Preserve records, consumed reservations and grants together,
including unknown outcomes, expired approvals, attempt fences and completed results.
A host needing scale should migrate the complete logical image to its own durable
store implementing the same atomic contract; there is no retention API here.

Deleting records, rotating to an empty file or evicting expired approvals can erase
replay/idempotency protection. Any host retention decision requires external proof
that old intents cannot return and downstream duplicate protection remains valid;
a lease or approval expiry is insufficient. No automatic eviction occurs.

Restore checks structural links in both directions: every grant-bearing record has
its matching grant and consumed reservation, with identical binding/approval expiry;
every consumed reservation points back to that record. Grantless records have zero
approval expiry. Unconsumed grants are valid. Expired historical grants remain valid
snapshot data; current dispatch eligibility is checked separately. These checks do
not authenticate plain JSON or detect a coherently forged image. Trusted storage,
backup provenance, access control and any integrity protection remain host duties.

Unapproved claims must have an empty `GrantID`; inconsistent claims now fail with
`invalid_claim` before dispatch/mutation. Clear irrelevant grant IDs in host inputs.
Recovery cannot change the original approval mode for the same operation binding;
a downgrade/upgrade fails with `binding_mismatch`, preserving the old reservation.
