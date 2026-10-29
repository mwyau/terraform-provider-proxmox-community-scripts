# Crash-safe Community Scripts creation: required protocol

**Design only. Create is intentionally disabled in this revision.**

The Terraform Plugin Framework can persist returned state after an error, but Create errors taint resources, and a controller crash can occur *after* the installer creates the Proxmox guest yet *before* the provider returns state. Therefore, a Terraform state lock or a per-host `flock` does not guarantee exactly-once script execution.

References: [Create](https://developer.hashicorp.com/terraform/plugin/framework/resources/create), [Read](https://developer.hashicorp.com/terraform/plugin/framework/resources/read), [Diagnostics](https://developer.hashicorp.com/terraform/plugin/framework/diagnostics).

## Host-side protocol required before enabling Create

1. Identify a guest by canonical cluster-scoped `kind/vmid`; carry an attempt UUID and canonical request digest. Pin immutable **40-character commit SHAs** for Community Scripts and Core. Moving branches are allowed for discovery, never for execution.
2. Check full cluster VMID inventory, selected immutable script blob, retired status, exact kind, node, and current journal before execution. Any occupied VMID fails. Do not auto-adopt.
3. Hold an **actual cluster-wide exclusion mechanism** across controllers and nodes. A host-local `flock` is insufficient when requests target different PVE nodes. A first release may require one configured coordinator host for all creates, with a root-owned per-VMID lock on that coordinator.
4. Before starting the installer, durably persist a protected host-side write-ahead journal (e.g. `/var/lib/proxmox-community-scripts/operations/<kind>/<vmid>/`). It records `prepared`, attempt UUID, request digest, refs and node. Use atomic rename and fsync of both file and directory; credentials must never appear in journal or logs.
5. Under the lock, recheck cluster VMID availability immediately before starting the installer. Use an audited fixed launcher, no shell interpolation of user values, no TTY, bounded execution time, protected source checkout and no password logging. Mark `running` durably before invoking the pinned installer **at most once for that journal key**.
6. Verify postconditions through the Proxmox API: exact VMID, type, node, and strong script identity (plus LXC updater content where supported). A successful script exit is not sufficient. Durably mark `verified` only after these checks. Do not assert an application version without an app-specific detector.
7. An SSH disconnect, timeout, failed installer, unexpected VMID, API outage or host crash leaves the journal `running` or `uncertain`. Never re-run, auto-clean, or auto-import such a resource. Require inspection and explicit recovery. A failed Terraform Create may be tainted; Delete must not destroy the guest by default.
8. On Read, remove Terraform state **only if** a complete successful cluster listing proves VMID absence. API failure, node mismatch, conflicting provenance or partial inspection must preserve state and report an error.

The journal reduces **automatic duplicate execution**. It is not an exactly-once guarantee for a nontransactional third-party installer.

## Recovery matrix

| Failure | Response |
| --- | --- |
| Crash before journal is durable | Installer has not started; retry may be safe after fresh preflight |
| Crash after `prepared`, before launch | No automatic retry; operator reviews journal and guest state |
| SSH loss during installation | Inspect host processes, journal and *all* cluster VMIDs |
| Guest created, Terraform loses state | Confirm identity, then explicit `terraform import` |
| Unexpected VMID selected | Fail; identify and repair manually |
| Installer reports success with identity conflict | Fail; preserve guest and journal |
| API unavailable during Read | Error; retain state |
| Verified guest genuinely absent | Remove stale state; future Create still guarded |
| Guest migrated nodes | Refresh node from cluster-wide identity; no new guest |
| Terraform destroy in retain mode | Remove tracking only, not the LXC/VM |

## Recovery operator procedure

1. Stop automatic applies. Inspect PVE inventory, tasks, guest config, relevant upstream app entrypoints, and journal. Ensure no child installer is still active on any candidate node.
2. If the guest is complete and script identity is verified, reconcile the declaration and use `terraform import communityscripts_guest.<name> lxc/<vmid>` (or `vm/<vmid>`), then review `terraform plan`.
3. If the guest is incomplete or identity conflicts, repair/retire it with independent operator approval. Do **not** delete its journal simply to permit retries.
4. If no guest remains, explicitly close the prior uncertain attempt before authorizing a fresh unique attempt UUID.

## Acceptance gates

- Successful first create and second no-op on isolated Proxmox.
- Two controllers and two nodes racing same VMID: no duplicate installation.
- Fault injection before/after journal durability, during execution, after guest allocation, after postflight, before state return.
- Nonzero exit, timeout, SSH failure, disconnected controller, unexpectedly remapped VMID, source retirement and conflicting provenance.
- No automatic retry or destructive rollback after uncertain creation.
- Explicit import and documented repair; live PVE version/storage evidence.
- No privileged installer CI on GitHub-hosted runners.
