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

## Public naming compatibility (bpg/proxmox conventions)

| Proxmox concept | Terraform guest resource | Ansible `application` facade | Notes |
| --- | --- | --- | --- |
| HTTPS origin | provider `endpoint` | `endpoint` | Same as bpg |
| API token | provider `api_token` | `api_token` | Combined `user@realm!tokenid=secret`; sensitive |
| TLS override | provider `insecure` | `insecure` | Default false; leave TLS verification enabled |
| Node | `node_name` | `node_name` | Exact PVE node name |
| Guest ID | `vm_id` | `vm_id` | One cluster-global ID for LXC or VM |
| Guest kind / script | `kind`, `script` | Same | Community Scripts-specific selectors |
| CPU/memory | `settings.cores`, `settings.memory` | Same | Flat creation map, MiB memory; not bpg nested CPU/memory blocks |
| Root disk pool | `settings.datastore_id` | Same | Original Core storage variable |
| Network | `settings.bridge`, `settings.vlan_id`, `settings.mac_address`, `settings.mtu` | Same | Single installer network interface |
| LXC privilege | `settings.unprivileged` | Same | `true` maps to `var_unprivileged=1` |
| VM start | `settings.started` | Same | Maps to Core `VM_START=yes/no` |
| Source pins | `scripts_ref`, `core_ref` | Same | Immutable SHA for installation |
| Software updates | `update_request_id`, `update_scripts_ref`, `update_backup_storage` | Same | Different lifecycle/approval semantics |
| Installation variables | `settings`, `guest_defaults`, `script_vars` | Same | Protected script-specific overrides |
| Guest access | Not yet implemented | `ssh`, `cloud_init` | Do not assume every VM supports Cloud-Init |
| Adoption override | Not implemented | `script_override` | No automatic trust of unknown script provenance |
| Lifecycle | Terraform CRUD/plan/import | `action=inspect/present/reconcile/update` | Never let both tools manage the same VMID |

The Terraform provider uses `ssh_target` for a privileged single-host installation runner; this is **not** equivalent to bpg's general `ssh` configuration block. Terraform state `id` remains computed, e.g. `lxc/120`, while `vm_id = 120` is explicit input. Unlike bpg, this provider does not auto-allocate VM IDs or reproduce bpg's full Proxmox resource schema. Lower-level Ansible modules retain their established native module parameters, not the public facade names.

## Normalized creation keys — exact 34/14 mapping

Both ecosystems use the *same normalized key* under `settings`, not the raw `var_*` / `VM_*` name. The Go contract maps the 34 LXC keys:

`cores memory disk_size hostname bridge vlan_id datastore_id template_storage os os_version unprivileged ipv4 gateway ipv6_method mac_address mtu dns searchdomain timezone tags protection nesting tun fuse keyctl gpu mknod mount_fs apt_cacher apt_cacher_ip inherit_host_ca sdn_vnet http_proxy http_no_proxy`

and the 14 VM keys:

`cores memory disk_size hostname bridge vlan_id datastore_id mac_address mtu cpu_type machine started disk_cache timezone`

to the Ansible Core environment mappings in `internal/contract/contract.go`. The canonical versioned specification is `contracts/guest.v1.json`, authored in the Ansible repository and copied here. `scripts/check_ansible_contract.py` verifies the JSON copies against the immutable Ansible SHA in `contracts/ansible.sha`, both source maps, both public schemas and the absence of temporary aliases on every Terraform CI run. Conversions also have shared golden tests.

Important conversions:
- LXC `unprivileged=true` means `var_unprivileged=1`; false means 0.
- LXC boolean toggles `tun`, `fuse`, etc. normalize to `yes/no`; flags `nesting`, `keyctl`, `mknod` use `1/0`.
- VM `disk_size=8` becomes `VM_DISK_SIZE=8G`, and `disk_cache=none` becomes `VM_DISK_CACHE=cache=none,`.
- No `var_*`, `VM_*` or host-control environment names may be passed through `script_vars` when a normalized field or engine control owns them.
- `guest_defaults` precedence: explicit `settings` > `guest_defaults[kind]` > `guest_defaults.all` > original script defaults. Defaults never reconcile existing guest hardware automatically.

Provider `settings` and `script_vars` are string maps. Use `"2048"` or `"true"` rather than depending on Terraform implicit scalar conversion. Any secrets entered via `script_vars` can still be stored in **Terraform state**, even with sensitive UI redaction. Use an encrypted, access-controlled state backend; for credentials supplied to upstream scripts, prefer a dedicated ephemeral secret-delivery mechanism in a future release.

## Remaining release gaps

The CI plan-time inspector does not validate PVE storage, bridge, VLAN existence, hardware restart behavior, selected application's interactive flows, Cloud-Init VM-specific hooks, or all external installer dependencies. Runtime execution, updates, and deletion must be tested on disposable Proxmox before release. Existing guest import still requires identity evidence; unknown or contradictory provenance is not silently overridden. LXC `swap` and VM `sockets` are Ansible *reconcile-only* options, not part of the 34/14 normalized creation mapping; Terraform support remains a separate task.


## bpg/proxmox-aligned public conventions (October 2026)
The current public provider/facade inputs are `endpoint`, `api_token` (combined `user@realm!tokenid=secret`), `insecure` (false unless explicitly set), `node_name`, and **one `vm_id` for LXC and VM alike**. The normalized creation `settings` names `datastore_id`, `vlan_id`, `mac_address`, `unprivileged`, and VM `started` translate to their respective upstream Core variables. Internal Ansible modules use native API arguments; both external interfaces reject superseded names. Provider `ssh_target`, creation pins, script variables, and approval gates remain Community Scripts-specific. `settings` remains a flat typed-as-string creation map, **not** a clone of bpg's separate disk/network/CPU blocks. This section supersedes older descriptions of `ctid`/`vmid` or separate provider tokens.


## Shared recipe scan and advisory status
The read-only upstream recipe scanner, retirement reports and advisory `recipes.json` status index now live in [proxmox-community-scripts-iac-common](https://github.com/mwyau/proxmox-community-scripts-iac-common/tree/feat/recipe-scanner-status). Each scan records the exact ProxmoxVE and Core Git revisions and potential prompts/VM field references. A scan result does **not** certify successful unattended installation.

The Terraform provider intentionally retains `internal/compat` as an independent per-guest, pinned-source, fail-closed check during planning. It does not download or trust a moving recipe status file before installation. Never treat an advisory status as a replacement for source checks or explicit approval of root script execution. No shared Python package or external runtime service is required.
