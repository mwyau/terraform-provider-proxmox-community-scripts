# Native provider implementation plan

## Scope and status
The objective is a native Go Terraform/OpenTofu provider complementing bpg/proxmox, not a wrapper around Ansible. The source Ansible feature branch is `mwyau/ansible-proxmox-community-scripts:feat/collection-foundation` at inspected commit `3797a68`. Its compatibility scanner, normalized variable mapping, provenance rules, and safe `present/reconcile/update` semantics are useful evidence, but **its live PVE tests had not passed when reviewed**.

## Required resource contract
1. `communityscripts_guest`: immutable node/VMID/kind/script + creation-only source pins/settings, observed status and provenance, import by kind/VMID.
2. `Create`: orchestrate fixed host launcher through SSH, pinned installer, durable journal, cluster VMID check, post-create identity verification. No duplicate retries.
3. `Read`: API refresh; migration aware; fail closed on ambiguous absence or mismatch.
4. `Update`: explicit, limited CPU/memory reconciliation and grow-only disk where tested; software updates through request IDs, never implicitly from source ref changes.
5. `Delete`: retain by default; explicit, guarded destructive delete after verifying kind/script and absence of partial/uncertain install; never delete on mismatch.
6. Independent inspect/catalog data sources may follow once lifecycle is stable.

## Interrupted creation
See `docs/INTERRUPTED_CREATION.md` and `AGENTS.md`. Required on-host stages: `prepared → running → verified`; unexpected exit `uncertain`. Each transition durable (fsync). A journal for the VMID exists before any side effect. Collision or stale journal stops execution; preserve journal even when Terraform state is lost. Recovery is manual inspection + explicit import, not automatic retry. Concurrent hosts must use one coordinator until distributed locking is proven. Terraform will not make installer side effects transactional.

## Security
Two credentials/privileges: least-privilege PVE API token for API; host root via audited launcher for installer/updater/deletion. Enforce known_hosts SSH checking and `BatchMode=yes`; fixed command with JSON request on stdin; never pass secrets as command arguments. Root launcher location is a fixed absolute path. Signed binary supply chain is a future release gate. Default TLS verification.

## Acceptance checklist
- [x] initial Go provider, import skeleton, read-only API, CI compilation.
- [x] import hydration and read-only provenance; API-failure and identity checks. Live imported-guest test remains pending.
- [x] durable host runner, root-checked CLI, fsync journal, single-coordinator lock, and mocked fault-injection tests.
- [x] guarded pinned LXC and VM creation paths with core field mappings and postflight unexpected-VMID detection. **Live script compatibility/retirement checks remain release blockers.**
- [x] API-backed CPU/memory reconciliation and explicit grow-only disk resizing with config digest. **Pending/restart semantics need live tests.**
- [x] pinned LXC updater with request-ID journal, backup storage requirement, updater slug verification and hardware-drift alarm.
- [x] guarded deletion with retain-default, stopped-state and identity checks, operator opt-in, and journal tombstone.
- [x] documentation, HCL samples, Go mocked API/runner tests, race-enabled test suite, Terraform schema validation and pinned-source `terraform plan` verification.
- [x] 34 LXC + 14 VM field mappings, secure `script_vars`, and layered `guest_defaults` with Ansible mapping drift check.
- [x] source-level compatibility and retirement heuristics for **selected** pinned scripts; inspect-only, not execution certification.
- [ ] guest `ssh` and VM `cloud_init` parity (safe public-key staging, supported image checks), import `script_override` policy.
- [ ] LXC swap and VM sockets reconcile parity, detailed restart/pending config behavior.
- [ ] stronger script compatibility evidence from selected pinned LXC/VM families and automated corpus scans on newer upstream revisions.
- [ ] self-hosted disposable PVE live test matrix: creation, interrupted process, second plan, VMID remap, update, destroy, import, PVE 8/9, storage backends.
- [ ] registry packaging, signed releases and release gates (only after live tests).

## Out of scope without independent evidence
- Automatic cleanup of arbitrary partial guests, cryptographically attested provenance, universal application-version detection, hidden disk shrink, changing VMID, active VM hardware mutation without restart plan, or executing upstream scripts on public CI runners.

## Implementation checkpoint (October 29, 2026)

Branch: `feat/go-provider-foundation`. Source and tests are checked by GitHub Actions on Ubuntu; no Proxmox privileged installer/updater/destructor is executed in public CI. The Go implementation covers Create/Read/Update/Delete/import but remains **experimental and not production-complete** pending real PVE integration, coordinator-hardening, script compatibility, signed runner delivery and crash tests against a disposable host. Do not mark any live-test item complete based on green mocked CI.

## Interface governance and dual-tool workflow
- [x] Canonical `api_host`, `validate_certs`, `update_backup_storage` without legacy aliases; Ansible `ctid` for LXC / `vmid` for VM and distinct `update_scripts_ref`.
- [x] Versioned Ansible-owned `contracts/guest.v1.json` with Terraform mirror; CI compares public names and all 34/14 mappings. Breaking changes require version bump and coordinated PRs.
- [ ] Enforce shared conversion fixtures across both languages in CI and block mismatched credential/guard semantics; increment contract schema for breaking changes.
- [ ] Live-PVE ownership handoff tests: exported Terraform inventory to Ansible in-guest tasks, plus explicit import and state removal.

## Public identity correction (2026-10-29)
The shared contract requires `ctid` for LXC or `vmid` for VM, mutually exclusive. Terraform's computed `id` remains the type-qualified import/state identity. internal PVE operations still take a normalized numeric VMID.
