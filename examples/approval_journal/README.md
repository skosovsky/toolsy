# Trusted local approval and durable replay

This ordinary CLI host uses its own subject/scope types, a restricted Session,
bound approval and the local durable adapter. There is no agent loop. It appends
one JSON receipt as the example external effect and stores the complete outcome.

Run from the repository root with an existing absolute directory owned only by
the host, on a supported local filesystem:

```sh
go run ./examples/approval_journal -directory /absolute/trusted-directory -operation first-intent
go run ./examples/approval_journal -directory /absolute/trusted-directory -operation first-intent -approve
go run ./examples/approval_journal -directory /absolute/trusted-directory -operation first-intent
```

The first call reports a pending challenge without writing a receipt. The second
is an explicit trusted local operator approval of that bound challenge. The last
replays the stored result without another receipt, even after a process restart.
Reuse the operation ID only for repeated delivery; use a new ID for a genuinely
new action. Changing the note under the old ID conflicts rather than silently
performing a second action.

`-approve` is not a remote authentication protocol. A production host must
authenticate subject and approver, bind the UI response to the immutable
challenge, manage issuer identity, current ACL, secret/dependency freshness and
continuation storage. Never derive approval from model input or free-text pause.
The demonstration identity is deliberately fixed; do not copy it as authentication.

If the process dies after receipt append but before outcome persistence, the
journal reports unknown/in-progress and refuses a new grant or blind retry. The
host must inspect/reconcile against authoritative external evidence using
`ReconcileOperation`, then replay through the current Session. This example does
not automatically infer proof from a file or issue retry permission. Filesystem
append is not a distributed exactly-once contract.

Protect the directory from other writers and symlinks; own access controls,
encryption, bounded growth, retention and lock-file lifecycle. Do not delete or
replace a live journal to force a retry. The sample is not a secure sandbox.
