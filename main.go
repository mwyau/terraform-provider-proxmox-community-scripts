package main

import (
 "context"
 "log"
 "github.com/hashicorp/terraform-plugin-framework/providerserver"
 "github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/provider"
)
func main() {
 if err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
  Address: "registry.terraform.io/mwyau/communityscripts",
 }); err != nil { log.Fatal(err) }
}
