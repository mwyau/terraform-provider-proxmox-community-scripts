# Development example only. Provider is not published; use a local dev_override.
terraform {
  required_providers {
    communityscripts = {
      source = "mwyau/proxmox-community-scripts"
    }
  }
}
variable "api_token_id" {
  type      = string
  sensitive = true
}
variable "api_token_secret" {
  type      = string
  sensitive = true
}
variable "scripts_sha" {
  type = string
}
variable "core_sha" {
  type = string
}
provider "communityscripts" {
  endpoint         = "https://pve1.example.internal:8006"
  api_token_id     = var.api_token_id
  api_token_secret = var.api_token_secret
  ssh_target       = "automation@pve1.example.internal"
}
resource "communityscripts_guest" "adguard" {
  node   = "pve1"
  vmid   = 120
  kind   = "lxc"
  script = "adguard"

  scripts_ref                = var.scripts_sha
  core_ref                   = var.core_sha
  accept_unverified_install  = true
  allow_hardware_updates     = true
  disk_key                   = "rootfs"
  settings = {
    cores     = "2"
    memory    = "2048"
    disk_size = "8"
    bridge    = "vmbr0"
    storage   = "local-lvm"
  }

  # Default: Terraform destroy removes *state tracking only*.
  deletion_policy = "retain"
}
