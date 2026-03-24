# Wazero Sandbox Adapter

`wazero` runs a precompiled WASI guest interpreter and exposes it as a single
LLM-safe language. The generic `exec_code` tool should never expose `wasm`
directly; instead, configure this adapter with the text language your guest
interpreter understands, such as `jq` or `rego`.
