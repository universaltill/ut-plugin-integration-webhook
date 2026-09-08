# Code review: auto-tag-release.yml (release-on-merge automation rollout)

**Date:** 2026-09-08
**Card:** ut-docs#1700 (mechanical rollout, continued from ut-docs#1694)
**Author:** scrum-master pipeline (cloud cycle, `lane:cloud-41`), on behalf of Farshid Mirza

## What changed

Added `.github/workflows/auto-tag-release.yml`, copied byte-for-byte from
the canonical, independently-reviewed copy in `ut-plugin-tax-de`
(`docs/code-reviews/2026-09-07-auto-tag-release-workflow-1694.md`). No
repo-specific deviation was needed or made.

## Live drift this repo is actually in — real end-to-end proof expected

This repo has **no git tags at all** — it has never had a tagged release
(`manifest.json`'s `version` is `1.0.0`, and no `v*` ref exists on
origin). Merging this workflow is expected to create the repo's
first-ever tag, `v1.0.0`, and dispatch a real `release.yml` run on the
first push to `main` that carries it — the same "never tagged at all"
proof case the previous batch demonstrated for `ut-plugin-tax-uk`.
DevOps must verify this actually happens.

## Independent review (fresh-context Sonnet subagent, per `complexity:easy` routing)

Verdict: **SAFE TO MERGE**, no findings. The subagent independently ran
`diff` against the canonical source (byte-identical), parsed the copied
YAML (clean), confirmed `manifest.json` at repo root with the valid
semver `version` above, confirmed `release.yml` is tag-triggered with
`workflow_dispatch` inputs named exactly `channel`/`publish` (no
adaptation needed), confirmed no conflicting tag/release automation
elsewhere and no `pull_request_target` exposure, and confirmed `main` is
this repo's actual default/integration branch.

## Verification performed

- `diff` against canonical source: byte-identical.
- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/auto-tag-release.yml'))"` — parses.
- Confirmed `manifest.json` version and `release.yml` dispatch input names via direct file inspection.
- DevOps to confirm the live effect after merge: new tag `v1.0.0` created
  on `main`, and a `release.yml` run dispatched and green.

## Non-goals confirmed out of scope

- Changing `release.yml`/`ci.yml` (no input-name mismatch found).
- The remaining `ut-plugin-*` repos in ut-docs#1700's scope (tracked on that issue).
