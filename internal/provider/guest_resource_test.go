package provider

import (
 "testing"
 "github.com/hashicorp/terraform-plugin-framework/types"
)
func TestModelValidation(t *testing.T){
 m:=guestModel{Node:types.StringValue("pve1"),Kind:types.StringValue("lxc"),VMID:types.Int64Value(120),Script:types.StringValue("adguard"),DeletionPolicy:types.StringValue("retain")}
 if e:=m.validate();e!=nil{t.Fatal(e)}
 m.DeletionPolicy=types.StringValue("destroy-all");if e:=m.validate();e==nil{t.Fatal("bad destroy policy accepted")}
 m.DeletionPolicy=types.StringNull();m.VMID=types.Int64Value(12)
 if e:=m.validate();e==nil{t.Fatal("bad VMID accepted")}
}
func TestNoDefaultDelete(t *testing.T){
 m:=guestModel{DeletionPolicy:types.StringNull()}
 if !m.DeletionPolicy.IsNull(){t.Fatal("default must retain")}
}
