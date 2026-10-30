package contract

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestFieldCounts(t *testing.T) {
	if len(LXCFields) != 34 || len(VMFields) != 14 {
		t.Fatalf("drift: %d LXC, %d VM fields", len(LXCFields), len(VMFields))
	}
}
func TestMappingsAndConversions(t *testing.T) {
	env, e := Environment("lxc", map[string]string{"cores": "2", "memory": "2048", "unprivileged": "true", "ipv6_method": "auto", "nesting": "false", "tUN": "yes"}, nil)
	if e == nil {
		t.Fatalf("unknown variable accepted: %v", env)
	}
	env, e = Environment("lxc", map[string]string{"cores": "2", "memory": "2048", "unprivileged": "true", "nesting": "false", "tun": "yes"}, map[string]string{"APP_OPTION": "x"})
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]string{"var_cpu": "2", "var_ram": "2048", "var_unprivileged": "1", "var_nesting": "0", "var_tun": "yes", "APP_OPTION": "x"}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("got %v, want %v", env, want)
	}
	vm, e := Environment("vm", map[string]string{"disk_size": "8", "disk_cache": "none", "started": "false", "machine": "q35"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if vm["VM_DISK_SIZE"] != "8G" || vm["VM_DISK_CACHE"] != "cache=none," || vm["VM_START"] != "no" {
		t.Fatalf("bad VM normalization %v", vm)
	}
}
func TestRejectOverrideAndInjection(t *testing.T) {
	for _, k := range []string{"PATH", "BASH_ENV", "LD_PRELOAD", "VM_RAM", "var_ctid", "var_cpu", "COMMUNITY_SCRIPTS_CORE_DIR", "APP-INVALID"} {
		if err := ScriptVars(map[string]string{k: "x"}); err == nil {
			t.Errorf("accepted %s", k)
		}
	}
	for _, tc := range []map[string]string{{"gpu": "undefined"}, {"vlan_id": "4095"}, {"ipv6_method": "static"}, {"cpu_type": "qemu64"}, {"memory": "64"}} {
		kind := "lxc"
		if _, ok := tc["cpu_type"]; ok {
			kind = "vm"
		}
		if err := ValidateSettings(kind, tc); err == nil {
			t.Errorf("accepted %v", tc)
		}
	}
	if err := ScriptVars(map[string]string{"APP_TOKEN": strings.Repeat("a", 32770)}); err == nil {
		t.Fatal("script_vars length accepted")
	}
}
func TestDefaultsPrecedence(t *testing.T) {
	d := map[string]map[string]string{"all": {"cores": "2", "memory": "1024"}, "lxc": {"memory": "2048"}}
	got, e := ResolveDefaults("lxc", d, map[string]string{"cores": "4"})
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]string{"cores": "4", "memory": "2048"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults=%v", got)
	}
}

func TestAllAnsibleFieldsReachRawCoreEnvironment(t *testing.T) {
	lxc := map[string]string{
		"cores": "2", "memory": "2048", "disk_size": "8", "hostname": "my-lxc", "bridge": "vmbr0", "vlan_id": "30",
		"datastore_id": "local-lvm", "template_storage": "local", "os": "debian", "os_version": "13",
		"unprivileged": "false", "ipv4": "192.0.2.10/24", "gateway": "192.0.2.1", "ipv6_method": "auto",
		"mac_address": "02:00:00:00:00:01", "mtu": "1500", "dns": "1.1.1.1", "searchdomain": "example.test",
		"timezone": "America/New_York", "tags": "lab", "protection": "true", "nesting": "1", "tun": "yes",
		"fuse": "false", "keyctl": "0", "gpu": "false", "mknod": "1", "mount_fs": "nfs,cifs",
		"apt_cacher": "false", "apt_cacher_ip": "proxy.local:3142", "inherit_host_ca": "true",
		"sdn_vnet": "vnet1", "http_proxy": "http://proxy.example:3128", "http_no_proxy": "localhost,127.0.0.1",
	}
	vm := map[string]string{
		"cores": "2", "memory": "2048", "disk_size": "8", "hostname": "testvm", "bridge": "vmbr0", "vlan_id": "30",
		"datastore_id": "local-lvm", "mac_address": "02:00:00:00:00:02", "mtu": "1500", "cpu_type": "host",
		"machine": "q35", "started": "false", "disk_cache": "none", "timezone": "America/New_York",
	}
	for _, c := range []struct {
		kind     string
		settings map[string]string
		names    map[string]string
	}{
		{"lxc", lxc, LXCFields}, {"vm", vm, VMFields},
	} {
		if len(c.settings) != len(c.names) {
			t.Fatalf("%s fixture covers %d of %d", c.kind, len(c.settings), len(c.names))
		}
		env, e := Environment(c.kind, c.settings, map[string]string{"APP_OPTION": "enabled"})
		if e != nil {
			t.Fatalf("%s: %v", c.kind, e)
		}
		if len(env) != len(c.names)+1 {
			t.Fatalf("%s mapping lost field: %v", c.kind, env)
		}
		for k, v := range c.names {
			if _, found := env[v]; !found {
				t.Errorf("%s key %s => %s not emitted", c.kind, k, v)
			}
		}
	}
}

func TestVersionedSharedGoldenCases(t *testing.T) {
	var shared struct {
		Version int               `json:"schema_version"`
		LX      map[string]string `json:"lxc_fields"`
		VM      map[string]string `json:"vm_fields"`
		Cases   []struct {
			Kind     string            `json:"kind"`
			Settings map[string]string `json:"settings"`
			Expected map[string]string `json:"expected_env"`
		} `json:"encoding_cases"`
	}
	data, e := os.ReadFile("../../contracts/guest.v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(data, &shared); e != nil {
		t.Fatal(e)
	}
	if shared.Version != 1 || !reflect.DeepEqual(shared.LX, LXCFields) || !reflect.DeepEqual(shared.VM, VMFields) {
		t.Fatal("Go field maps diverged from the versioned canonical contract")
	}
	for _, tc := range shared.Cases {
		got, e := Environment(tc.Kind, tc.Settings, nil)
		if e != nil {
			t.Fatalf("%s: %v", tc.Kind, e)
		}
		if !reflect.DeepEqual(got, tc.Expected) {
			t.Fatalf("%s got=%v expected=%v", tc.Kind, got, tc.Expected)
		}
	}
}
