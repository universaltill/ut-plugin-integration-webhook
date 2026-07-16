# ut-plugin-integration-webhook — rules for working in this repo

One plugin per repo (docs repo ADR-0009). This is the **generic webhook / ERP
connector** (`canonical_type: integration`, `runtime: wasm`) and the reference
implementation + **template** for the SAP and Microsoft Dynamics / LS Central
connectors (ADR-0014).

- Offline-first is the contract (ADR-0003): checkout must never wait on or fail
  because of the ERP. Delivery is best-effort — queue undelivered sales in
  plugin storage and retry on the next invocation. The plugin **always exits 0**.
- The wasm module is plain Go `GOOS=wasip1 GOARCH=wasm` (scripts/build.sh);
  host functions imported from module `ut` (see docs repo
  `reference/plugin-host-functions.md`). Buffer ABI: data calls return the FULL
  length, retry with a bigger buffer if it exceeds the cap; negatives are
  errors (-1 not found, -2 denied, -3 internal, -4 invalid).
- Permissions `events:receive`, `net:*`, `storage` are declared in the manifest —
  keep code and manifest in sync. `net:*` because the target host is unknown
  until install-time settings (review-gated, ADR-0006).
- ERP-specific connectors are clones that override **one seam** — `postSale` in
  `src/main.go` (transform the sale JSON to the target shape) — and keep the
  queue/retry/settings plumbing unchanged.
- Standards & decisions live in the docs repo (`adr/`, ADR-0007 document-first).
  Behaviour changes update the README in the same session.
