# 2026-08-09 — Release-pipeline template fixes (ut-docs#166)

## Context
ut-docs#166 (found during ut-docs#15's review, bringing this repo's CI up to
the `ut-plugin-payment-sumup` template) flagged three pre-existing,
byte-identical defects shared with `ut-plugin-payment-sumup` and
`ut-plugin-faq` (the reference implementation `ut-docs/for-plugin-developers.md`
tells new plugin authors to copy):

1. `latest.tar.gz.sha256` — and, found during this fix's own review, the
   versioned artifact's own `.sha256` sidecar too — recorded a
   `dist/`-prefixed path instead of the bare filename, so
   `sha256sum -c` fails for any self-hoster who downloads the artifact +
   checksum pair into one directory.
2. `scripts/approve.sh` reused `MARKETPLACE_UPLOAD_TOKEN` (a vendor-upload
   credential) as the bearer token for the marketplace's admin
   review/approve endpoints. Fails closed today, not currently exploitable.
3. `ci.yml`'s validate job ran no `go vet`/`gofmt` gate.

## Changes
- **`scripts/package.sh`**: the `sha256sum "$OUT"` line now `cd`s into
  `dist/` and hashes the bare filename, fixing the versioned artifact's
  sidecar (found during review — the original ticket only named the
  `latest.tar.gz` copy).
- **`.github/workflows/release.yml`**: the "Create GitHub Release" step
  regenerates `latest.tar.gz.sha256` against the renamed
  `dist/latest.tar.gz` directly instead of copying the versioned `.sha256`
  file verbatim.
- **`scripts/approve.sh`**: now prefers `MARKETPLACE_ADMIN_TOKEN`, falling
  back to `MARKETPLACE_UPLOAD_TOKEN` (unchanged current behavior) when
  unset. **Client-side prep only** — ut-cloud's `authorizeStaff` accepts
  only the upload-token value today, so a genuinely distinct
  `MARKETPLACE_ADMIN_TOKEN` would 401 every call. Filed
  **universaltill/ut-docs#496** for the real fix; comments in both files
  say explicitly not to provision a distinct secret until #496 ships.
- **`.github/workflows/ci.yml`**: added `go vet` + `gofmt -l .`, run
  cross-compiled (`GOOS=wasip1 GOARCH=wasm`) for parity with the sumup
  template's fix even though this repo's `main.go` carries no `wasip1`
  build tag today (host vet already covered it; cross-compiling is
  harmless and keeps both repos' `ci.yml` mechanically identical).

## Independent review
Fresh-context Opus review (different model from the implementer), run once
across all three repos together (identical diffs). No blockers; three
should-fix items applied before commit — see
`ut-plugin-payment-sumup`'s twin record for the full findings detail,
verification methodology, and rationale (this repo's diff is byte-identical
to sumup's for every fix except the Go-specific cross-compile framing note
above).

## Verification
- `scripts/build.sh && scripts/validate.sh && scripts/package.sh` — green.
- `GOOS=wasip1 GOARCH=wasm go vet ./...` and `gofmt -l .` — clean at HEAD.
- Reproduced the self-hoster checksum failure pre-fix and confirmed both
  the versioned sidecar and the `latest.tar.gz` pair pass `sha256sum -c`
  post-fix.
- Token-selection logic verified in isolation across all 4
  admin/upload-token combinations.
- `bash -n` and YAML parse on every edited file.

## Deliberately out of scope
- ut-cloud-side distinct admin credential — universaltill/ut-docs#496.
- Sweeping the other `ut-plugin-*` repos — universaltill/ut-docs#497.
- Adding `go test` to CI here: this repo's `main.go` is wasm-import-only
  and doesn't build under the host `go test` toolchain
  (`missing function body` on bodyless `//go:wasmimport` funcs) — a
  pre-existing condition, not something this ticket's scope covers.
