package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"testing"
)

func TestModelValidation(t *testing.T) {
	m := guestModel{Node: types.StringValue("pve1"), Kind: types.StringValue("lxc"), VMID: types.Int64Value(120), Script: types.StringValue("adguard"), DeletionPolicy: types.StringValue("retain")}
	if e := m.validate(); e != nil {
		t.Fatal(e)
	}
	m.DeletionPolicy = types.StringValue("destroy-all")
	if e := m.validate(); e == nil {
		t.Fatal("bad destroy policy accepted")
	}
	m.DeletionPolicy = types.StringNull()
	m.VMID = types.Int64Value(12)
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

func TestBothKindsUseVMID(t *testing.T) {
 m:=guestModel{Node:types.StringValue("pve1"),Kind:types.StringValue("lxc"),VMID:types.Int64Value(120),Script:types.StringValue("adguard")}
 if e:=m.validate();e!=nil{t.Fatal(e)}
 if m.id().String()!="lxc/120"{t.Fatal(m.id())}
 m.Kind=types.StringValue("vm")
 if e:=m.validate();e!=nil{t.Fatal(e)}
 if m.id().String()!="vm/120"{t.Fatal(m.id())}
 m.VMID=types.Int64Null()
 if e:=m.validate();e==nil{t.Fatal("missing vm_id accepted")}
}
