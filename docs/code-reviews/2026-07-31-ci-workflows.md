# 2026-07-31 — CI/release workflows added

**Card:** universaltill/ut-docs#15 (split from a composite card that also
covered `AUTO_APPROVE` revisit, deferred to ut-docs#165, and `ut-plugin-themes`,
which turned out to already be resolved — that repo is archived and superseded
by three per-theme repos that each already carry their own CI).

## What shipped

This repo had `src/main.go` + `scripts/build.sh` but no CI and wasn't
publishable — unlike every other `ut-plugin-*` repo. Brought up to the same
template `ut-plugin-payment-sumup` already ships:

- `scripts/validate.sh`, `scripts/package.sh`, `scripts/publish.sh`,
  `scripts/approve.sh` — mechanically templated, adapted because this
  manifest wires via `hooks: [{event, action}]` rather than the `entries`
  array payment/theme plugins use.
- `.github/workflows/ci.yml` (build+validate+package on push/PR) and
  `.github/workflows/release.yml` (tag-triggered package+publish+dev-marketplace
  auto-approve, same shape as sumup's).

No behavior change to the plugin's runtime logic (`src/main.go` untouched).

## Independent review (different-model subagent, Opus)

Given the exact diff scope, the repo's `CLAUDE.md`, ADR-0006 (plugin trust
chain), and instructed to actually run the scripts and try to break them
rather than just read the diff. Full findings and verified repro commands
in the agent's report (kept in this pipeline's run log); summary:

**Fixed as part of this card:**
- **F1** (non-blocking, highest value): `validate.sh` capped `hooks` at
  exactly 1, transposed from the payment-plugin "exactly 1 tender" rule with
  no equivalent invariant for integration hooks. ADR-0014 clones may
  legitimately subscribe more than one event (e.g. `stock.adjusted` for
  inventory sync alongside `sale.completed`) — the first such clone would
  have inherited permanently-red CI from this template. Verified: adding a
  second hook to the manifest failed validate with `got 2` before the fix,
  passes after.
- **F2** (non-blocking): entrypoint check was existence-only; a 0-byte or
  garbage `bin/plugin.wasm` passed validate and got packaged. Now asserts
  non-empty + wasm magic bytes (`\x00asm`). Verified: an emptied
  `bin/plugin.wasm` now fails validate with a clear message.
- **F3** (non-blocking, guards a real failure mode): `permissions` check was
  truthiness-only; a clone dropping `storage` would still pass CI, and this
  plugin's own offline-queue durability guarantee (ADR-0003/ADR-0014) depends
  on that permission — losing it means queued sales are silently dropped
  (`saveQueue` logs and the plugin still exits 0, by design, so nothing
  surfaces). Now asserts `{events:receive, net:*, storage}` ⊆ permissions.
  Verified: dropping `storage` now fails validate.
- **F5** (nitpick): `ci.yml` had no `permissions:` block, inheriting the
  org/repo default `GITHUB_TOKEN` scope for a job that needs none. Added
  `permissions: contents: read`.
- **F7** (nitpick): a comment in `publish.sh` carried a real internal
  hostname as an example value; genericized to `marketplace.example.com`
  since this repo is the clone source for future ERP-connector repos.
- **F8** (nitpick, this repo's own `CLAUDE.md` rule): README's "Develop"
  section still only listed `build.sh` after four scripts and two workflows
  were added. Updated.

**Deferred, not this card's scope (pre-existing, shared with the already-shipped
`ut-plugin-payment-sumup` template, byte-identical in this repo — not a new
regression introduced here):**
- **F4**: `release.yml`'s `latest.tar.gz.sha256` is a straight copy of the
  original artifact's checksum file, whose content still names the original
  path — `sha256sum -c` against the advertised `latest.tar.gz` fails for a
  self-hoster. Reproduced by the reviewer against this repo's workflow; same
  defect exists in sumup's.
- **F9**: `approve.sh` reuses `MARKETPLACE_UPLOAD_TOKEN` (a vendor-upload
  credential) as the bearer for admin-review endpoints — token-scope
  conflation. Fails closed if unset (curl `--fail`), so not exploitable as-is,
  but should use a distinct admin credential. Byte-identical to sumup's script.
- **F6**: neither this repo's nor sumup's CI runs `go vet`/`go test`/`gofmt`
  — CI proves "compiles to wasm, manifest parses," nothing about `postSale`
  behavior. Worth backlog for the connector repos generally.
- Filed as **universaltill/ut-docs#166** ("plugin release-pipeline template
  bugs shared by sumup and webhook connector: broken latest.tar.gz.sha256,
  admin/vendor token conflation in approve.sh, no Go tests in CI") rather than
  scope-creeping this card.

**ADR-0006 assessment:** `approve.sh` is byte-identical to sumup's; nothing in
this repo touches signing/key material or POS-side verification
(`manifest_verifier.go` untouched). The only automated link is the human
review decision, gated on the same opt-in `AUTO_APPROVE` repo variable sumup
already carries — parity with the existing accepted dev-marketplace risk, not
a new weakening.

**Injection/secrets:** clean — no `${{ }}` interpolation inside any `run:`
script body, `pull_request` (not `_target`) so fork PRs get a read-only token,
full grep for credential-shaped strings found only `${{ secrets.* }}`/
`${{ vars.* }}`/`${{ github.token }}` and the (now-fixed) F7 comment. No real
shop/client names anywhere.

## Verified beyond automated tests

Ran `scripts/build.sh` → `validate.sh` → `package.sh` locally end-to-end, and
re-ran the full negative-test matrix the independent reviewer used (corrupt
manifest, missing/extra hooks, wrong canonical_type/runtime, missing
permission, empty/garbage wasm module) — each fails closed with the right
message, and the fixed cases (2 hooks; missing `storage`; empty wasm) behave
correctly after the fixes. Inspected the packaged tarball with `tar -tzf` —
no `./`-prefixed or path-traversal members. `gofmt -l .` and `go vet ./...`
clean.

## Safe to merge

Yes — independent reviewer's verdict was "safe to merge as-is" before these
were even applied; the four fixes above raise the bar further and are all
verified.
