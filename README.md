# Terraform provider for Proxmox Community Scripts

Experimental **native Go** Terraform/OpenTofu provider for Community Scripts guests. It complements [the Ansible collection](https://github.com/mwyau/ansible-proxmox-community-scripts) and [bpg/proxmox](https://github.com/bpg/terraform-provider-proxmox), rather than competing with their general-purpose Proxmox resource management.

**Implementation:** guarded LXC/VM creation, API-backed identity inspection and import, explicitly authorized CPU/memory and grow-only disk reconciliation, opt-in request-ID-based LXC software updates, and retain-by-default or explicitly approved deletion. All privileged creation, update and deletion operations require a separately installed root-owned PVE runner. **Not production validated:** no live Proxmox installation, update or deletion has been tested. No Registry release exists.

## Architecture and safety guarantees

- One authoritative Terraform owner per VMID. Do not simultaneously manage a Community Scripts guest with this provider and bpg's LXC/VM resource.
- Proxmox HTTPS API token performs inventory reads and approved compute/disk configuration writes; host root execution takes place through `ssh -T` and a fixed audited runner command. SSH uses BatchMode and strict known-host checking.
- Both Community Scripts and Core are pinned to **full, immutable 40-character commit hashes** for installation. No source code is downloaded and executed in CI. Source pinning is not a safety audit.
- The host runner uses one durable per-guest journal plus root-protected file locking **on the designated coordinator host**. A journal that exists blocks automatic re-installation, even if Terraform crashed after guest allocation.
- Existing resources require strong script identity evidence from Proxmox descriptions or supported VM tags. An unknown/conflicting slug is not automatically adopted. LXC software updates additionally require exact `/usr/bin/update` slug evidence.
- Any uncertain create or update requires human investigation, not an automatic retry. Terraform and third-party installation scripts cannot offer transactional exactly-once creation.
- **Only one PVE coordinator node per cluster is supported by this version.** Host-local locking does not serialize arbitrary other hosts/controllers. The runner refuses a request for a different node. Never deploy this against multiple PVE nodes concurrently.
- Destroy retains the guest by default. Destructive deletion requires both explicit Terraform flags, an exact identity match, a stopped guest, and a verified host journal (or separate explicit import-delete approval).
- No automatic guest shutdown/reboot, arbitrary SSH access to guests, silent storage movement, disk shrink, or VM application updater.

See [AGENTS.md](AGENTS.md), [PLAN.md](PLAN.md), [docs/INTERRUPTED_CREATION.md](docs/INTERRUPTED_CREATION.md), and [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for the lifecycle design, cross-repo naming contract, scan evidence, and remaining gaps.

## Development / installation

Requires Go 1.23+, Terraform or OpenTofu, and a Proxmox API token. The provider is not published to a registry:

```sh
go mod download
go test -race ./...
go vet ./...
go build -o terraform-provider-proxmox-community-scripts .
go build -o communityscripts-runner ./cmd/communityscripts-runner
```

Use Terraform's `dev_overrides` to load your local provider binary during development. Example `~/.terraformrc` (set an absolute path to the directory containing the built provider binary):

```hcl
provider_installation {
  dev_overrides {
    "mwyau/proxmox-community-scripts" = "/absolute/path/to/build"
  }
  direct {}
}
```

For privileged operations, install the runner separately on a **disposable Proxmox test host**:

```sh
sudo install -o root -g root -m 0755 communityscripts-runner /usr/local/sbin/communityscripts-runner
```

The runner uses `/var/lib/proxmox-community-scripts/operations` for durable root-owned journals and pinned-source checkouts. It requires `pvesh`, `git`, `bash`, and `pct`/`qm`. Provide a dedicated SSH principal with only passwordless `sudo -n /usr/local/sbin/communityscripts-runner execute`, not unrestricted root access; protect SSH keys outside Terraform state. Ensure the SSH client trusts the coordinator's host key. The Proxmox API token must have sufficient cluster inventory visibility and resource permissions.

**Risk:** the runner executes externally maintained root installers. Do not install it or grant its sudo permission on production hosts until you have completed the disposable integration and audit gates in PLAN.md.

## Read-only verification at terraform plan

New resources with known pinned 40-character `scripts_ref` and `core_ref` are inspected in `ModifyPlan`. The provider fetches the exact script and the relevant shared Core source, rejects missing or clearly retired entrypoints and absent requested Core variable mappings, and warns about possible prompts and VM-specific variable consumption. **No script is executed by plan.** Network failure fails closed, while unknown input values are explicitly deferred to apply-time inspection. Passing means **source checks passed, not that a live installation succeeded**.

```sh
terraform plan
# Review the warnings and pinned revisions. No apply is needed to inspect scripts.
```

See [examples/plan-verify.tf](examples/plan-verify.tf). Since script variables may contain credentials, be aware that Terraform can store `script_vars` values in encrypted/access-controlled state even though the schema marks them sensitive. The root runner never persists these values in its on-host journal.

## Resource declaration

See [examples/lifecycle.tf](examples/lifecycle.tf) and [examples/import.tf](examples/import.tf). Proposed provider source is `mwyau/proxmox-community-scripts`; resource type is `communityscripts_guest` (the provider's internal type prefix is `communityscripts`).

Creation requires `scripts_ref`, `core_ref`, `accept_unverified_install=true`, a free VMID, and `ssh_target` pointing to the selected coordinator. The complete **34 LXC / 14 VM Ansible-compatible normalized settings** vocabulary is implemented in `internal/contract`, with settings precedence `settings` > `guest_defaults[kind]` > `guest_defaults.all` > upstream defaults. `script_vars` is a protected, bounded creation-only environment map (64 entries, 32 KiB). `settings` and `disk_key` are optional. See [the exact field names and conversions](docs/COMPATIBILITY.md). Missing values are not overridden; inherited defaults never reconcile existing guests.

`terraform import communityscripts_guest.adguard lxc/120` (or `vm/120`) associates an **existing** guest. It does not run the script. It requires strongly identifying metadata in the PVE API; existing LXCs with updater evidence but no description cannot yet be imported through API-only inspection.

**In-place hardware:** change only `settings.cores`, `settings.memory`, or grow `settings.disk_size`, with `allow_hardware_updates=true`. For LXC disk growth, `disk_key` defaults to `rootfs`; for VM disks supply the exact existing `scsiN`, `virtioN`, `sataN`, or `ideN` key. No disk creation/shrink/move is supported. CPU/memory changes may remain pending until restart.

**Application updates:** after initial create, set or change `update_request_id` together with a pinned `update_scripts_ref` and `update_backup_storage`. A host journal prevents executing the same successful request ID twice. This is **request-level** idempotency, not an application-version detector. The updater can still change hardware, fail partially, or have other side effects; verify the guest after each update. VM application updates are not supported.

**Destroy:** omit `deletion_policy` (or use `retain`) to remove Terraform tracking without deleting the guest. For actual deletion set `deletion_policy="delete"` and `confirm_destroy=true`, apply those settings first, stop the guest yourself, then plan destroy. A guest imported without a verified creation journal additionally requires `allow_imported_destroy=true`. The host runner's durable journal prevents auto-destroy of uncertain partial creation. Never use destructive replacement to fix an identity conflict.

## Testing and status

GitHub Actions checks the versioned Ansible contract and both public interfaces for drift, compiles and tests Go with race detection, validates the Terraform provider schema, and runs a real **read-only Terraform plan** against pinned source files. It does not execute Community Scripts or mutate Proxmox. Strict committed formatting is required. Tests cover successful/failed creation, journal replay, race-relevant collisions, extra VMIDs, LXC updater evidence, application update replay, identity conflict, retain/delete guards, and disk growth safety.

**Known gaps / release blockers:** live PVE 8/9 testing (LXC + VM, stopped/running, bridge/VLAN/storage, source compatibility), robust cluster-wide coordinator enforcement, installer cancellation behavior, source retirement detection, and release packaging/signing. Until these are addressed, use disposable hosts only.

MIT licensed. Not affiliated with Proxmox Server Solutions or Community Scripts maintainers.

## Contract synchronization and division of ownership

The canonical `contracts/guest.v1.json` lives in the Ansible repository; an identical copy is committed here. CI compares against the immutable Ansible commit recorded in `contracts/ansible.sha`, and fails if the versions, normalized mappings (34 LXC / 14 VM), or public input names diverge. Update this pin in coordinated PRs when changing the contract. Make interface changes in a coordinated pair of PRs, updating both implementations and tests; bump the contract version for breaking changes. Do **not** add deprecated or compatibility aliases. The versioned contract is a shared schema, not shared runtime state.

For a VMID declared in Terraform, use Terraform for create, hardware changes, updates and destroy. Use Ansible for disjoint in-guest configuration (including its guest SSH connection), inspection, or separately owned PVE host services; do not also use Ansible `action: present/reconcile/update` on the same VMID. Ownership transfer is explicit import/removal, never duplicate state management. See [docs/OWNERSHIP.md](docs/OWNERSHIP.md).

### Kind-specific creation identifiers

Set `ctid` for `kind="lxc"`, or `vmid` for `kind="vm"`; never both. The upstream LXC Core uses `var_ctid`, while PVE APIs use the cluster-scoped numeric VMID internally. Terraform's `id` is **computed** as `lxc/120` or `vm/121` for state and import, not a user-provided creation alias. This matches the Ansible public application role.
