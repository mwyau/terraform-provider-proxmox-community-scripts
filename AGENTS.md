# AGENTS.md — provider invariants

Goal: manage Proxmox Community Scripts LXC/VM guests natively through Terraform/OpenTofu without confusing an installer invocation with a transactional guest lifecycle.

## Architecture and ownership
- This repository owns Community Scripts guest provisioning, observed identity, narrowly scoped updates, and explicit destruction. `bpg/proxmox` owns unrelated PVE resources. Never have two managed Terraform resources claim the same VMID.
- Prefer direct Proxmox HTTPS API tokens for reads and approved hardware writes; PVE-host root execution requires a separately installed, audited launcher invoked over SSH. No application guest SSH is required.
- Host scripts are third-party root code. Pin **both** upstream `community-scripts/ProxmoxVE` and `community-scripts/core` by immutable full 40-character Git SHAs; inspect upstream compatibility separately. A pinned SHA does not prove the script is safe or unattended.
- VMID is cluster-scoped, script identity and kind immutable, migration must not imply creation. No silent adoption or VMID reassignment. Unknown/conflicting provenance always fails closed.
- Terraform import is explicitly supported for verified existing guests. API metadata and LXC updater entrypoints are evidence, *not* cryptographic proof.
- Creation-only defaults never modify existing hardware. Application updates and host-resource reconciliation are distinct explicit actions.

## Crash/interruption and state
- Terraform state and state locking **do not** guarantee exactly-once side effects. Create may be interrupted after guest allocation but before state save; a failed Create is tainted and may otherwise rerun.
- Before invoking a root installer, the host runner must hold an exclusive stable lock and durably write a root-protected journal (atomic rename and fsync file+directory). A prior journal blocks automatic re-execution, including `prepared` or `running` attempts.
- Mark `running` durably before launching; leave failed/unknown attempts for operator recovery. Never rerun, delete, or automatically import a partially installed guest; successful exit alone does not prove installation.
- Only an exact guest VMID/kind/node/slug check following installer execution qualifies as verified. Verify through the PVE API/host; no fake installed-application version prediction.
- A host-local lock is not a cluster-wide lock. Until a distributed coordinator exists, require all installs within the same PVE cluster to use one host-side coordinator; refuse nonlocal targets. Document this limitation.
- During Read, API errors, missing permissions, ambiguous results, type/slug changes, or uncertain absence must preserve prior Terraform state and report errors rather than trigger destructive replacement.
- Default destroy is `retain`; explicit delete mode must verify identity and stop state, require direct approval, and refuse unknown/failed partial creation.
- Software updates use a unique request ID with a persistent success marker. An update request is not a reliable installed-version probe, and an error can leave partial changes.
- Secrets must not appear in the on-host journal, command line, Terraform output, logs, or CI fixtures.

## Implementation and testing
- Native Go Terraform Plugin Framework. Avoid Ansible execution inside provider; reuse its tested behavior and normalized settings mapping where feasible.
- All lifecycle paths need fake API/SSH/PVE runner tests, collision, wrong-kind/provenance, journal state transitions, timeout, retries, and interrupted-creation fault injection.
- Never run downloaded upstream scripts in public GitHub Actions. Real integration tests require disposable isolated PVE, explicit operator opt-in, and recorded PVE/storage/source revisions.
- Prefer safe partial functionality over falsely promising transactional semantics. When a phase is unavailable, fail explicitly, document exactly what is missing, and do not silently emulate it.
- Keep `PLAN.md`, `docs/INTERRUPTED_CREATION.md`, README, and tests synchronized with actual implemented behavior. Do not claim live validation from mocked CI.

## Git work
- Continue in `feat/go-provider-foundation`; do not overwrite or force-push existing work.
- Commit small batches with test results and track any unsupported/untested scenarios.
- Do not change the default branch without an explicit request.
