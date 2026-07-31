# Webhook / ERP Connector (`com.universaltill.integration-webhook`)

Mirrors every completed sale from **Universal Till** into any external system.
On each `sale.completed` event it POSTs the sale payload to a URL you configure,
with an auth header you configure. It works today with any middleware or
integration bus, and it is the **reference implementation and template** for the
ERP-specific connectors — **SAP** and **Microsoft Dynamics 365 / LS Central** —
which are clones that only swap the payload transform (ADR-0014).

## What it does

`canonical_type: integration`, `runtime: wasm`. It subscribes to the
`sale.completed` hook and runs in-process (wazero, WASI command, plain Go
`GOOS=wasip1`). For each completed sale it POSTs the raw `SaleCompletedEvent`
JSON — sale id, receipt number, currency, subtotal/discount/tax/total (integer
minor units), customer/register/cashier, line items and payments — **verbatim**
as the request body.

## Settings

Configured per install in the plugin settings editor (all scope `global`):

| Key | Default | Meaning |
|---|---|---|
| `endpoint_url` | `""` | The URL each sale is POSTed to. **Empty = disabled**: a fresh install is a clean no-op, never an error. |
| `auth_header` | `Authorization` | Name of the auth header to send. |
| `auth_value` | `""` | Value for that header (e.g. `Bearer <token>`). **Empty = the auth header is omitted.** |

The request always carries `Content-Type: application/json`.

## Offline-first / retry queue

Checkout must never wait on, or fail because of, an ERP (ADR-0003). Delivery is
therefore best-effort and asynchronous from the till's point of view:

- If a POST fails — a host/network error **or** any non-2xx response — the sale
  payload is **queued** in the plugin's own storage (a JSON array under
  `delivery_queue`).
- On **every** invocation the connector **flushes the queue first**: it re-POSTs
  each queued sale, drops the ones that now return 2xx, and keeps the rest. Then
  it handles the current sale.
- The queue is **bounded** (cap ~200 entries); the oldest entries are dropped
  past the cap so storage can't grow without limit.
- Sales carry a stable id + receipt number, so redelivery is idempotent for the
  receiving system.
- The plugin **always exits 0** — a connector must never fail the tender path;
  errors are logged instead.

## Host functions used

Imported from the `ut` module (see the docs repo
`reference/plugin-host-functions.md`): `log_write`, `storage_get`,
`storage_set`, `http_request`, `settings_get`. Requires permissions
`events:receive`, `net:*` (the target host is unknown until install-time
settings, so the connector declares the wildcard and is review-gated
accordingly, ADR-0006) and `storage`.

## Template for ERP connectors

The SAP and Dynamics/LS connectors are clones of this plugin. Everything —
settings plumbing, the offline queue, retry-on-next-invocation, exit-0
discipline — stays identical. They override **one seam**: `postSale` in
`src/main.go`, which for this generic connector forwards the sale JSON verbatim.
An ERP connector transforms that JSON into the target shape (SAP IDoc/BAPI/OData,
Dynamics/LS Business Central OData) before the POST.

## Develop

```bash
scripts/build.sh      # GOOS=wasip1 GOARCH=wasm go build → bin/plugin.wasm (prints byte size)
scripts/validate.sh   # manifest schema + wasm entrypoint checks
scripts/package.sh    # dist/<id>_<version>_universal.tar.gz + .sha256
scripts/publish.sh    # upload the packaged artifact to the marketplace
scripts/approve.sh    # dev-marketplace only: auto-approve + sign the upload
```

CI (`.github/workflows/ci.yml`) runs build+validate+package on every push/PR;
`.github/workflows/release.yml` packages, publishes, and (dev marketplace
only, `AUTO_APPROVE` repo variable) auto-approves on a `v*` tag push.
