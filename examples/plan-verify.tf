# Read-only plan-time compatibility probe. Do not apply this example.
terraform {
  required_providers {
    communityscripts = {
      source = "mwyau/proxmox-community-scripts"
    }
  }
}
provider "communityscripts" {
  endpoint = "https://127.0.0.1:8006"
  api_token = "ci@pve!test=unused-for-plan"
}
resource "communityscripts_guest" "inspect_only" {
  node_name = "pve1"
  kind    = "lxc"
  vm_id = 120
  script  = "adguard"

  scripts_ref               = "bc6ab719828f9d52556af3a17ceff2f01c1a17d6"
  core_ref                  = "5acac4483be88990354dd4e3778e2cdce4df428e"
  accept_unverified_install = true

  guest_defaults = {
    all = {
      cores  = "2"
      memory = "1024"
    }
    lxc = {
      os = "debian"
    }
  }
  settings = {
    memory = "2048"
  }
  script_vars = {
    APP_TEST_FLAG = "1"
  }
  deletion_policy = "retain"
}
