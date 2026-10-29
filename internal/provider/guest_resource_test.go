package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestCreateFailsClosed(t *testing.T) {
	r := &GuestResource{}
	var resp resource.CreateResponse
	r.Create(context.Background(), resource.CreateRequest{}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create must fail without any installer implementation")
	}
}

func TestUpdateFailsClosed(t *testing.T) {
	r := &GuestResource{}
	var resp resource.UpdateResponse
	r.Update(context.Background(), resource.UpdateRequest{}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Update must not mutate a guest")
	}
}

func TestDeleteRetainsGuest(t *testing.T) {
	r := &GuestResource{}
	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{}, &resp)
	if resp.Diagnostics.HasError() || len(resp.Diagnostics.Warnings()) == 0 {
		t.Fatal("Delete must retain the real Proxmox guest and warn")
	}
}
