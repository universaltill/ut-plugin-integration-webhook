#!/usr/bin/env bash
# Validates the integration-connector manifest: marketplace-required fields
# (id/name/semver/permissions/locales), wasm runtime with a built entrypoint,
# and exactly one hooks entry (this connector wires via hooks, not entries).
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - <<'PY'
import json, os, re, sys
m = json.load(open("manifest.json"))
errs = []
if not re.match(r'^[a-z0-9]+([.-][a-z0-9]+)*$', m.get("id","")): errs.append("bad id")
if not m.get("name"): errs.append("missing name")
if not re.match(r'^\d+\.\d+\.\d+', m.get("version","")): errs.append("bad version")
if not m.get("permissions"): errs.append("missing permissions")
if not m.get("locales"): errs.append("missing locales")
if m.get("runtime") != "wasm": errs.append("runtime must be 'wasm' (ADR-0001)")
else:
    ep = (m.get("entrypoint") or "").lstrip("./")
    if not ep.endswith(".wasm"): errs.append("wasm runtime needs a .wasm entrypoint")
    elif not os.path.isfile(ep): errs.append(f"module not found: {ep} (run scripts/build.sh)")
if m.get("device_arch") != "any": errs.append("device_arch must be 'any'")
if m.get("canonical_type") != "integration": errs.append("canonical_type must be 'integration'")
hooks = m.get("hooks", [])
if len(hooks) != 1:
    errs.append(f"expected exactly 1 hooks entry, got {len(hooks)}")
else:
    if not hooks[0].get("event"): errs.append("hooks entry missing event")
    if not hooks[0].get("action"): errs.append("hooks entry missing action")
if errs:
    print("FAIL: " + "; ".join(errs)); sys.exit(1)
print(f"ok {m['id']} v{m['version']}")
PY
