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

See [AGENTS.md](AGENTS.md), [PLAN.md](PLAN.md) and [docs/INTERRUPTED_CREATION.md](docs/INTERRUPTED_CREATION.md) for the immutable design and recovery protocol.

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

## Resource declaration

See [examples/lifecycle.tf](examples/lifecycle.tf) and [examples/import.tf](examples/import.tf). Proposed provider source is `mwyau/proxmox-community-scripts`; resource type is `communityscripts_guest` (the provider's internal type prefix is `communityscripts`).

An LXC creation requires `scripts_ref`, `core_ref`, `accept_unverified_install=true`, a missing VMID, and `ssh_target` pointing to the same node as the requested guest. Both `settings` and `disk_key` are optional. The supported normalized creation settings are `cores`, `memory` (MiB), `disk_size` (GiB), `hostname`, `bridge`, `storage`, and `vlan`. Inputs map to the LXC `var_*` or VM `VM_*` Core contract. Unsupported inputs and raw arbitrary installer environment variables are refused.

`terraform import communityscripts_guest.adguard lxc/120` (or `vm/120`) associates an **existing** guest. It does not run the script. It requires strongly identifying metadata in the PVE API; existing LXCs with updater evidence but no description cannot yet be imported through API-only inspection.

**In-place hardware:** change only `settings.cores`, `settings.memory`, or grow `settings.disk_size`, with `allow_hardware_updates=true`. For LXC disk growth, `disk_key` defaults to `rootfs`; for VM disks supply the exact existing `scsiN`, `virtioN`, `sataN`, or `ideN` key. No disk creation/shrink/move is supported. CPU/memory changes may remain pending until restart.

**Application updates:** after initial create, set or change `update_request_id` together with a pinned `update_scripts_ref` and `backup_storage`. A host journal prevents executing the same successful request ID twice. This is **request-level** idempotency, not an application-version detector. The updater can still change hardware, fail partially, or have other side effects; verify the guest after each update. VM application updates are not supported.

**Destroy:** omit `deletion_policy` (or use `retain`) to remove Terraform tracking without deleting the guest. For actual deletion set `deletion_policy="delete"` and `confirm_destroy=true`, apply those settings first, stop the guest yourself, then plan destroy. A guest imported without a verified creation journal additionally requires `allow_imported_destroy=true`. The host runner's durable journal prevents auto-destroy of uncertain partial creation. Never use destructive replacement to fix an identity conflict.

## Testing and status

GitHub Actions runs gofmt (temporary development normalization), `go vet`, `go test -race`, and `go build` against local fake APIs and host process adapters; it never executes Community Scripts. Tests cover successful/failed creation, journal replay, race-relevant collisions, extra VMIDs, LXC updater evidence, application update replay, identity conflict, retain/delete guards, and disk growth safety.

**Known gaps / release blockers:** live PVE 8/9 testing (LXC + VM, stopped/running, bridge/VLAN/storage, source compatibility), robust cluster-wide coordinator enforcement, installer cancellation behavior, source retirement detection, and release packaging/signing. Until these are addressed, use disposable hosts only.

MIT licensed. Not affiliated with Proxmox Server Solutions or Community Scripts maintainers.
