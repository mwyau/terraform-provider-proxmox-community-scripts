# Interrupted creation and durable host journal

**Implemented in Go, not yet live-PVE validated.** Terraform state alone does not guarantee exactly-once resource creation. A provider process may crash after the upstream script creates a guest but before Terraform receives state. A failed Create is tainted; blindly retrying can duplicate or collide with the guest.

## Current protocol

1. The provider calls the Proxmox HTTPS API and refuses an existing VMID, then dispatches a JSON request over verified non-interactive SSH to a fixed root-owned binary.
2. The host runner validates canonical kind, VMID, requested node, script, full immutable Git SHAs, approved normalized settings, and an explicit acknowledgement of unverified root code.
3. The runner takes a single root-protected `flock` on the **one designated coordinator host**. It rejects a journal that already exists for the requested VMID (regardless of phase), checks the Proxmox cluster inventory, and writes the `prepared` journal by temporary file + fsync + atomic rename + directory fsync.
4. It fetches both pinned upstream repositories, rechecks the VMID, and marks `running` durably **before** invoking the script with a fixed `bash` argv and validated Core environment.
5. After completion it inspects the cluster inventory for unexpected new VMIDs and positively verifies the requested guest's VMID, kind, node, and script evidence. Only then is the journal `verified`.
6. Source failures and installer failures leave the journal `uncertain`. SSH loss or process termination may leave `prepared` or `running`; all non-verified phases refuse retry. No deletion or automatic recovery is performed.
7. The provider separately inspects the API before writing Terraform state. Loss of state after a verified host create still requires manual inspection and Terraform import. Failed state writes do not trigger an automatic second installer run.

This is *at-most-once automatic invocation under the single host lock*, not exactly-once completion. A local `flock` does not protect against another PVE coordinator or somebody manually running scripts on another node. The runner refuses nonlocal node requests; users must configure a single coordinator host per cluster. Terraform state locks cannot replace this rule.

## Current update and delete behavior

LXC software update uses a pinned update script revision, explicit `update_request_id`, required backup storage, and strong description/tag plus updater slug evidence. The host journal marks a successful request ID as `verified` and suppresses repetition; a failed/interrupted request is not automatically retried. No generic application-version prediction is available.

Deletion requires a stopped guest, matching VMID/kind/node/script metadata, explicit Terraform opt-in, and a verified creation journal (or distinct imported-destroy approval). The runner journals `deleting` before `pct destroy` / `qm destroy`, then confirms the VMID is gone and writes `deleted`. A crash after destruction but before tombstone completion leaves an uncertain journal requiring manual inspection. Default Terraform deletion **retains** the guest.

Hardware changes use the Proxmox API; they are not part of the host execution journal. API failures can occur after partial application. Re-inspect guest config before retrying.

## Recovery procedure

1. Stop Terraform automated applies. Inspect the root journal at `/var/lib/proxmox-community-scripts/operations/<kind>/<vmid>/journal.json` and (for application updates) `updates/<request_id>.json`.
2. Inspect `pvesh get /cluster/resources --type vm`, the guest config/status, PVE tasks and any installer process. Confirm there is no running child process before further action.
3. If the guest exists and exact provenance is verified, reconcile HCL and use explicit `terraform import ... lxc/<vmid>` or `vm/<vmid>`. Do not clear the journal.
4. If the guest is incomplete or a different VMID appeared, repair/remove it by a separately approved operator process. A normal provider apply will refuse re-creation with the old journal.
5. For an absent guest after an uncertain attempt, preserve the incident record. There is no automatic journal reset workflow. Operator-run cleanup must have a reviewed protocol before authorizing a new immutable attempt.
6. On API failures or ambiguous inventory, Terraform Read deliberately preserves state and errors rather than assuming deletion. Use `terraform state rm` only after independent verification.

## Release gates still unmet

- Live disposable Proxmox LXC + VM install/upgrade/delete; pinned Core script entrypoints and retired scripts.
- Two-controller race on one coordinator; attempt from different coordinators; remapped VMID.
- Forced crashes at every journal transition, after actual allocation, after API postflight and before Terraform state return.
- Verify behavior on Proxmox 8 and 9, multiple storage backends and supported script families.
- Hardening root runner distribution, command cancellation, process-group termination, logging redaction, and package signing.

References: [Framework Create](https://developer.hashicorp.com/terraform/plugin/framework/resources/create), [Diagnostics](https://developer.hashicorp.com/terraform/plugin/framework/diagnostics), [Read](https://developer.hashicorp.com/terraform/plugin/framework/resources/read).
