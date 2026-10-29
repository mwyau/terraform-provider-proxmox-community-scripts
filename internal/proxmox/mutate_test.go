package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComputeMutationUsesDigest(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "PUT" || r.URL.Path != "/api2/json/nodes/pve1/lxc/120/config" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if e := r.ParseForm(); e != nil {
			t.Error(e)
		}
		if r.PostForm.Get("cores") != "4" || r.PostForm.Get("memory") != "4096" || r.PostForm.Get("digest") != "hash" {
			t.Errorf("incorrect PUT body: %v", r.PostForm)
		}
		fmt.Fprint(w, `{"data":null}`)
	}))
	defer srv.Close()
	c, e := NewClient(srv.URL, "u@pam!token", "test", true)
	if e != nil {
		t.Fatal(e)
	}
	g := Guest{Kind: "lxc", Node: "pve1", VMID: 120, Digest: "hash"}
	if e = c.ApplyCompute(context.Background(), g, map[string]string{"cores": "4", "memory": "4096"}); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatalf("count %d", calls)
	}
	for _, m := range []map[string]string{{"swap": "512"}, {"cores": "0"}, {"memory": "128"}, {"cores": "foo"}} {
		if e = c.ApplyCompute(context.Background(), g, m); e == nil {
			t.Fatalf("accepted %v", m)
		}
	}
	if calls != 1 {
		t.Fatalf("invalid settings caused %d API writes", calls)
	}
}
