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
- [ ] import hydrates required attributes; tests for name/slug/node provenance and API absence.
- [ ] durable host runner, checked root/permissions, journal and lock; unit/fault injection.
- [ ] pinned LXC creation and VM creation; real Core mapping and retired script checks.
- [ ] CPU/RAM and disk grow-only reconciler with pending config handling.
- [ ] explicit request-ID application updater with backup requirements and no implicit reinstall.
- [ ] guarded deletion with intentional `retain` vs `delete` and partial-state protection.
- [ ] documentation, sample HCL and end-to-end mocked tests for each lifecycle.
- [ ] self-hosted disposable PVE live test matrix: creation, interrupted process, second plan, VMID remap, update, destroy, import, PVE 8/9, storage backends.
- [ ] registry packaging, signed releases and release gates (only after live tests).

## Out of scope without independent evidence
- Automatic cleanup of arbitrary partial guests, cryptographically attested provenance, universal application-version detection, hidden disk shrink, changing VMID, active VM hardware mutation without restart plan, or executing upstream scripts on public CI runners.
