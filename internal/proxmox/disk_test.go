package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDiskGiB(t *testing.T) {
	for _, tc := range []struct {
		cfg  string
		want float64
	}{
		{"local:vm-120-disk-0,size=8G", 8},
		{"local:vm-120-disk-0,size=1024M", 1},
		{"local:vm-120-disk-0,size=1T", 1024},
	} {
		got, e := DiskGiB(tc.cfg)
		if e != nil || got != tc.want {
			t.Fatalf("%s => %v %v", tc.cfg, got, e)
		}
	}
	if _, e := DiskGiB("local:vm-120-disk-0"); e == nil {
		t.Fatal("missing size accepted")
	}
}
func TestGrowDiskRefusesShrink(t *testing.T) {
	g := Guest{Kind: "lxc", VMID: 120, Node: "pve1", Disks: map[string]string{"rootfs": "local:disk,size=16G"}}
	c := &Client{}
	if e := c.GrowDisk(context.Background(), g, "rootfs", "8"); e == nil {
		t.Fatal("shrink accepted")
	}
	if e := c.GrowDisk(context.Background(), g, "mp0", "18"); e == nil {
		t.Fatal("other mountpoint accepted")
	}
	g.Kind = "vm"
	if e := c.GrowDisk(context.Background(), g, "", "20"); e == nil {
		t.Fatal("ambiguous VM disk accepted")
	}
}
func TestGrowDiskFormAndTask(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api2/json/nodes/pve1/lxc/120/resize" {
			if r.Method != "PUT" {
				t.Errorf("method %s", r.Method)
			}
			_ = r.ParseForm()
			if r.PostForm.Get("disk") != "rootfs" || r.PostForm.Get("size") != "20G" || r.PostForm.Get("digest") != "abcdef" {
				t.Errorf("form %v", r.PostForm)
			}
			fmt.Fprint(w, `{"data":"UPID:pve1:1"}`)
			return
		}
		if r.URL.Path == "/api2/json/nodes/pve1/tasks/UPID:pve1:1/status" {
			fmt.Fprint(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		http.Error(w, "unknown", 404)
	}))
	defer srv.Close()
	c, e := NewClient(srv.URL, "a@pam!test", "secret", true)
	if e != nil {
		t.Fatal(e)
	}
	g := Guest{Kind: "lxc", VMID: 120, Node: "pve1", Digest: "abcdef", Disks: map[string]string{"rootfs": "local:disk,size=8G"}}
	if e = c.GrowDisk(context.Background(), g, "rootfs", "20"); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatalf("expected resize+task, got %d", calls)
	}
	_ = url.Values{}
}
