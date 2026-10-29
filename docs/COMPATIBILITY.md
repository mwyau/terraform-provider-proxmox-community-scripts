# Community Scripts compatibility and shared variable contract

Last audit: 2026-10-29. Terraform branch `feat/go-provider-foundation`; Ansible branch `feat/collection-foundation` with canonical `contracts/guest.v1.json`.

## What Terraform plan verifies

For a **new** `communityscripts_guest` declaration with known `scripts_ref` and `core_ref`, `terraform plan` uses read-only HTTPS to retrieve:

- `ProxmoxVE/ct/<script>.sh` or `ProxmoxVE/vm/<script>.sh` at the **40-character pinned scripts commit**.
- `core/ui/defaults.func` or `core/pve/vm-core.func` at the **40-character pinned Core commit**.

The provider checks the script exists, contains the expected shared build/VM lifecycle entrypoint, is not an obvious retired/renamed exit-only stub, and that shared Core contains the normalized variables requested by `settings` and the resolved `guest_defaults` layers. VM-specific use of the requested fields and possible interactive prompts produce **warnings**.

Unknown refs, HTTP errors, absent entrypoints, missing shared Core variables, and invalid normalized settings are **plan errors**. The planner does not fetch or run arbitrary shell commands, contact Proxmox by SSH, create/modify guests, or claim that an application is unattended.

Terraform can run `plan` without `apply`:

```sh
terraform plan
# inspect diagnostics and selected immutable revisions; this DOES NOT install anything
```

The `examples/plan-verify.tf` example uses pinned source commits and mock PVE credentials. GitHub Actions validates it without a Proxmox host. Unknown values are deferred to apply-time verification and explicitly warned about; an unverified plan is not a deployment guarantee.

## Broad upstream scan (Ansible scanner, independent source evidence)

[Ansible upstream compatibility workflow](https://github.com/mwyau/ansible-proxmox-community-scripts/actions/runs/40909006370), October 29, 2026:

- Scanned scripts SHA: `3cdee53cefa2cb42e4ffb2669e0fa1ded6efb8c0`.
- Scanned Core SHA: `3c1ef2a14e6dbfc93e9200098d0abdeeba916d72`.
- 663 source entrypoints classified, including four retired scripts excluded from active results.
- Remaining 659: 557 shared engine, no app prompts detected; 14 update prompts only; 85 install-input review; 3 wizard prompt review.

The Ansible scanner ran Bash source syntax checks and heuristics on a pinned snapshot. **These categories are not measured installation success rates.** Some reads are ordinary text-processing loops rather than prompts, and complex dynamically sourced shell code can evade detection. VM per-script parameter consumption can vary even where the shared Core defines a variable. The current upstream inventory at `bc6ab719828f9d52556af3a17ceff2f01c1a17d6` contained 644 LXC and 19 VM `.sh` entrypoints; do not assume every file is active, currently installable, or supports all normalized options.

In particular, the `adguard` script has a `msg_menu` OS choice when `var_os` is absent; for unattended configuration, set `settings.os`. This source-specific consideration is **not** automatically resolved by having pinned SHAs.

Terraform's plan-time inspector is deliberately narrower than the repository-wide Ansible scanner; it checks the *selected* pinned script and does not certify the full catalog. Keep both scanners and their results distinct. The provider must not treat source syntax checks as permission to execute arbitrary upstream code.

## Public naming compatibility

| Ansible collection | Terraform | Semantics |
| --- | --- | --- |
| `api_host` | Same | Proxmox HTTPS origin |
| `api_token_id`, `api_token_secret` | Same | API credentials |
| `validate_certs` | Same | Defaults to true; no inverse alias |
| `kind`, `script`, `node` | Same | Guest type, script slug, chosen PVE node |
| `ctid` (LXC) / `vmid` (VM) | Same | Exactly one required; Proxmox still uses a cluster-wide numeric VMID internally |
| `scripts_ref`, `core_ref` | Same | Full immutable Git SHAs for creation |
| `update_scripts_ref` | Same | Separate immutable source revision for application software-update requests |
| `settings` | Same | Creation settings and explicit supported compute/disk updates |
| `guest_defaults.all/lxc/vm` | Same | Applied **only at Create**, not reconciled into existing guests |
| `script_vars` | Same | At most 64, 32 KiB; arbitrary valid app env names excluding protected/normalized Core names |
| `accept_unverified_install` | Same | Approval to run third-party root scripts |
| `disk_key` | Same | Disk chosen for grow-only operations |
| `update_request_id` | Same | Explicit idempotency key |
| `update_backup_storage` | Same | Update backup storage; no alias |
| `ssh`, `cloud_init` | Not yet available | Requires secure host key staging, per-image support and real integration tests |
| `script_override` | Not yet available | Unsafe identity overrides are deliberately refused |
| `update_apply`, `update_backup`, `update_accept_no_backup` | Not exposed | Terraform change to a request ID triggers update; backup storage is mandatory |
| `action=present/reconcile/update/inspect` | Native CRUD, plan and import | Terraform lifecycle is state-driven |
| No delete action in Ansible facade | `deletion_policy`, `confirm_destroy` | Retain by default; destructive opt-in only |

## Normalized creation keys — exact 34/14 mapping

Both ecosystems use the *same normalized key* under `settings`, not the raw `var_*` / `VM_*` name. The Go contract maps the 34 LXC keys:

`cores memory disk_size hostname bridge vlan storage template_storage os os_version privileged ipv4 gateway ipv6_method mac mtu dns searchdomain timezone tags protection nesting tun fuse keyctl gpu mknod mount_fs apt_cacher apt_cacher_ip inherit_host_ca sdn_vnet http_proxy http_no_proxy`

and the 14 VM keys:

`cores memory disk_size hostname bridge vlan storage mac mtu cpu_type machine start disk_cache timezone`

to the Ansible Core environment mappings in `internal/contract/contract.go`. The canonical versioned specification is `contracts/guest.v1.json`, authored in the Ansible repository and copied here. `scripts/check_ansible_contract.py` verifies the JSON copies, both source maps, both public schemas and the absence of temporary aliases on every Terraform CI run. Conversions also have shared golden tests.

Important conversions:
- LXC `privileged=true` means `var_unprivileged=0`; false means 1.
- LXC boolean toggles `tun`, `fuse`, etc. normalize to `yes/no`; flags `nesting`, `keyctl`, `mknod` use `1/0`.
- VM `disk_size=8` becomes `VM_DISK_SIZE=8G`, and `disk_cache=none` becomes `VM_DISK_CACHE=cache=none,`.
- No `var_*`, `VM_*` or host-control environment names may be passed through `script_vars` when a normalized field or engine control owns them.
- `guest_defaults` precedence: explicit `settings` > `guest_defaults[kind]` > `guest_defaults.all` > original script defaults. Defaults never reconcile existing guest hardware automatically.

Provider `settings` and `script_vars` are string maps. Use `"2048"` or `"true"` rather than depending on Terraform implicit scalar conversion. Any secrets entered via `script_vars` can still be stored in **Terraform state**, even with sensitive UI redaction. Use an encrypted, access-controlled state backend; for credentials supplied to upstream scripts, prefer a dedicated ephemeral secret-delivery mechanism in a future release.

## Remaining release gaps

The CI plan-time inspector does not validate PVE storage, bridge, VLAN existence, hardware restart behavior, selected application's interactive flows, Cloud-Init VM-specific hooks, or all external installer dependencies. Runtime execution, updates, and deletion must be tested on disposable Proxmox before release. Existing guest import still requires identity evidence; unknown or contradictory provenance is not silently overridden. LXC `swap` and VM `sockets` are Ansible *reconcile-only* options, not part of the 34/14 normalized creation mapping; Terraform support remains a separate task.
