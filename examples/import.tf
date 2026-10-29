# Example only; the provider is not yet published.
terraform {
 required_providers {
  communityscripts = { source = "mwyau/proxmox-community-scripts" }
 }
}
variable "proxmox_api_token_id" { type = string; sensitive = true }
variable "proxmox_api_token_secret" { type = string; sensitive = true }
provider "communityscripts" {
 endpoint         = "https://pve.example.net:8006"
 api_token_id     = var.proxmox_api_token_id
 api_token_secret = var.proxmox_api_token_secret
}
resource "communityscripts_guest" "adguard" {
 node="pve1"
 kind="lxc"
 vmid=120
 script="adguard"
}
