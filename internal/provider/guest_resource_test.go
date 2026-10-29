package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"testing"
)

func TestModelValidation(t *testing.T) {
	m := guestModel{Node: types.StringValue("pve1"), Kind: types.StringValue("lxc"), CTID: types.Int64Value(120), Script: types.StringValue("adguard"), DeletionPolicy: types.StringValue("retain")}
	if e := m.validate(); e != nil {
		t.Fatal(e)
	}
	m.DeletionPolicy = types.StringValue("destroy-all")
	if e := m.validate(); e == nil {
		t.Fatal("bad destroy policy accepted")
	}
	m.DeletionPolicy = types.StringNull()
	m.CTID = types.Int64Value(12)
	if e := m.validate(); e == nil {
		t.Fatal("bad VMID accepted")
	}
}
func TestNoDefaultDelete(t *testing.T) {
	m := guestModel{DeletionPolicy: types.StringNull()}
	if !m.DeletionPolicy.IsNull() {
		t.Fatal("default must retain")
	}
}

func TestCreationOptionsMatchAnsiblePrecedence(t *testing.T) {
	ctx := context.Background()
	defaults, ds := types.MapValueFrom(ctx, types.MapType{ElemType: types.StringType},
		map[string]map[string]string{"all": {"cores": "2", "memory": "1024"}, "lxc": {"memory": "2048", "os": "debian"}})
	if ds.HasError() {
		t.Fatalf("defaults: %v", ds)
	}
	settings, ds := types.MapValueFrom(ctx, types.StringType, map[string]string{"cores": "4"})
	if ds.HasError() {
		t.Fatalf("settings: %v", ds)
	}
	vars, ds := types.MapValueFrom(ctx, types.StringType, map[string]string{"APP_OPTION": "yes"})
	if ds.HasError() {
		t.Fatalf("script_vars: %v", ds)
	}
	m := guestModel{Kind: types.StringValue("lxc"), Settings: settings, GuestDefaults: defaults, ScriptVars: vars}
	effective, extras, e := m.creationOptions(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if effective["cores"] != "4" || effective["memory"] != "2048" || effective["os"] != "debian" || extras["APP_OPTION"] != "yes" {
		t.Fatalf("unexpected effective creation options %v extras %v", effective, extras)
	}
}

func TestCanonicalUpdateBackupStorage(t *testing.T) {
	m := guestModel{UpdateBackup: types.StringValue("backup")}
	if m.UpdateBackup.ValueString() != "backup" {
		t.Fatal("update backup storage not persisted")
	}
}

func TestKindSpecificIDs(t *testing.T) {
 base:=guestModel{Node:types.StringValue("pve1"),Kind:types.StringValue("lxc"),CTID:types.Int64Value(120),Script:types.StringValue("adguard")}
 if e:=base.validate();e!=nil{t.Fatal(e)}
 if base.id().String()!="lxc/120"{t.Fatalf("wrong LXC state ID %s",base.id().String())}
 base.VMID=types.Int64Value(121)
 if e:=base.validate();e==nil{t.Fatal("accepted CTID and VMID together")}
 base.CTID=types.Int64Null()
 if e:=base.validate();e==nil{t.Fatal("accepted VMID for LXC")}
 base.Kind=types.StringValue("vm")
 if e:=base.validate();e!=nil{t.Fatal(e)}
 if base.id().String()!="vm/121"{t.Fatalf("wrong VM state ID %s",base.id().String())}
 base.CTID=types.Int64Value(120)
 if e:=base.validate();e==nil{t.Fatal("accepted CTID on VM")}
 base.VMID=types.Int64Null()
 if e:=base.validate();e==nil{t.Fatal("accepted VM without VMID")}
}
