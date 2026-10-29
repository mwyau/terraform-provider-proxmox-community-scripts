# Terraform and Ansible: practical ownership

## One owner per VMID

**Terraform-owned guest** (recommended for declarative lab infrastructure):
1. Terraform `communityscripts_guest` creates and tracks the LXC/VM, its identity, hardware and explicit update request IDs.
2. The existing bpg/proxmox provider manages independent PVE objects (storage, networks, permissions, etc.) and may use read-only data sources for the guest. **Do not manage the same guest as both `communityscripts_guest` and a bpg managed LXC/VM resource.**
3. Ansible configures the guest's software (packages, config files, SSH keys) using a separate playbook and identity; LXC may use `community.proxmox.proxmox_pct_remote` without guest sshd. This playbook must not call the public Community Scripts `action: present`, `reconcile`, or `update` for the Terraform-owned VMID, nor change Terraform-managed hardware.
4. Inventory may be generated from `terraform output -json` or from read-only Proxmox API queries; check type, VMID and node identity before configuration.
5. Run `terraform plan` first for infrastructure changes, `terraform apply` if approved, then the Ansible guest-configuration playbook. Run separately or in a CI pipeline with distinct approval gates.

**Ansible-owned guest:** use `action: present/reconcile/update` as needed, and do not declare that VMID as a managed `communityscripts_guest` resource. Terraform may read it through a data source but must not run Create on it.

**Ownership transfer:** verify exact type, VMID, node, source identity and current hardware. From Ansible to Terraform, stop Ansible provisioning for the VMID and `terraform import communityscripts_guest.<name> lxc/<vmid>` (or `vm/<vmid>`). From Terraform to Ansible, disable Terraform mutation and explicitly remove Terraform state without deleting the real guest; ensure `deletion_policy=retain` and use a reviewed state removal workflow. Then enable Ansible changes. Do not let both controllers run during the transition.

## Shared interface, independent execution

The only common public parameter names are defined by the versioned `contracts/guest.v1.json`. Both tools keep native lifecycle semantics: Terraform manages state and plan/apply/destroy; Ansible offers explicit inspect/present/reconcile/update and check mode. `update_apply` and `install` remain Ansible approval controls, not Terraform state aliases.

**No hidden integration:** The Terraform provider does not automatically invoke Ansible. The Ansible collection does not read Terraform state or know Terraform ownership unless the operator supplies its inventory and workflow constraints. A shared field contract is not a locking protocol or coordination service.

## Release governance

Canonical contract: `mwyau/ansible-proxmox-community-scripts/contracts/guest.v1.json`, current schema v1. Terraform mirrors that file and validates both source mappings and public APIs in CI; the Ansible repository tests its own adapter against the same contract. Update both in paired PRs. Once published, pin contract versions to immutable release tags rather than development branch names. A breaking change increments the contract version, with no deprecated aliases retained in this pre-release stage.

No live Proxmox integration has been completed for this workflow.
