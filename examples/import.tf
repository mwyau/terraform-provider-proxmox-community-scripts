# Example only: import an existing, positively identified Community Scripts LXC.
# Provider is not yet published; configure a local dev_override.
terraform {
  required_providers {
    communityscripts = {
      source = "mwyau/proxmox-community-scripts"
    }
  }
}

variable "api_token" {
  type      = string
  sensitive = true
}

provider "communityscripts" {
  endpoint = "https://pve.example.net:8006"
  api_token = var.api_token
}

resource "communityscripts_guest" "adguard" {
  node_name = "pve1"
  kind   = "lxc"
  vm_id = 120
  script = "adguard"

  deletion_policy = "retain"
}

# After configuration is ready:
# terraform import communityscripts_guest.adguard lxc/120
