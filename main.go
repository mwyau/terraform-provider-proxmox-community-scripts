package main

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/provider"
	"log"
)

func main() {
	if err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/mwyau/proxmox-community-scripts",
	}); err != nil {
		log.Fatal(err)
	}
}
