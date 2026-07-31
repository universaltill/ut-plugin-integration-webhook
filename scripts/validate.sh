#!/usr/bin/env bash
# Validates the integration-connector manifest: marketplace-required fields
# (id/name/semver/permissions/locales), wasm runtime with a real built
# entrypoint, and at least one well-formed hooks entry (this connector wires
# via hooks, not entries) — ADR-0014 clones may subscribe more than one event
# (e.g. sale.completed + stock.adjusted), so this must not cap it at exactly 1.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - <<'PY'
import json, os, re, sys
m = json.load(open("manifest.json"))
errs = []
if not re.match(r'^[a-z0-9]+([.-][a-z0-9]+)*$', m.get("id","")): errs.append("bad id")
if not m.get("name"): errs.append("missing name")
if not re.match(r'^\d+\.\d+\.\d+', m.get("version","")): errs.append("bad version")
required_perms = {"events:receive", "net:*", "storage"}
perms = set(m.get("permissions") or [])
if not perms: errs.append("missing permissions")
elif not required_perms.issubset(perms): errs.append(f"missing required permissions: {sorted(required_perms - perms)}")
if not m.get("locales"): errs.append("missing locales")
if m.get("runtime") != "wasm": errs.append("runtime must be 'wasm' (ADR-0001)")
else:
    ep = (m.get("entrypoint") or "").lstrip("./")
    if not ep.endswith(".wasm"): errs.append("wasm runtime needs a .wasm entrypoint")
    elif not os.path.isfile(ep): errs.append(f"module not found: {ep} (run scripts/build.sh)")
    else:
        with open(ep, "rb") as f: head = f.read(4)
        if os.path.getsize(ep) == 0: errs.append(f"{ep} is empty (run scripts/build.sh)")
        elif head != b"\x00asm": errs.append(f"{ep} is not a wasm module (bad magic bytes)")
if m.get("device_arch") != "any": errs.append("device_arch must be 'any'")
if m.get("canonical_type") != "integration": errs.append("canonical_type must be 'integration'")
hooks = m.get("hooks", [])
if len(hooks) < 1:
    errs.append("expected at least 1 hooks entry, got 0")
else:
    for i, h in enumerate(hooks):
        if not h.get("event"): errs.append(f"hooks[{i}] missing event")
        if not h.get("action"): errs.append(f"hooks[{i}] missing action")
if errs:
    print("FAIL: " + "; ".join(errs)); sys.exit(1)
print(f"ok {m['id']} v{m['version']}")
PY
