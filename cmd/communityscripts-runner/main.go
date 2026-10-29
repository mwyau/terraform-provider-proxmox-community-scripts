// communityscripts-runner must be installed separately as root-owned mode 0755
// on an isolated PVE coordinator. It is NOT run in GitHub Actions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/runner"
)

func main() {
	// Do not resolve commands from an SSH-provided or user-controlled PATH.
 os.Setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin")
 if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "root required")
		os.Exit(1)
	}
	if len(os.Args) != 2 || os.Args[1] != "execute" {
		fmt.Fprintln(os.Stderr, "usage: communityscripts-runner execute")
		os.Exit(2)
	}
	f, err := os.Open("/etc/hostname")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hostname unavailable")
		os.Exit(1)
	}
	data, err := io.ReadAll(io.LimitReader(f, 256))
	f.Close()
	if err != nil {
		os.Exit(1)
	}
	var req runner.Request
	dec := json.NewDecoder(io.LimitReader(os.Stdin, 65536))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&req); err != nil {
		fmt.Fprintln(os.Stderr, "invalid request")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	r := runner.Runner{Dir: "/var/lib/proxmox-community-scripts/operations", Node: strings.TrimSpace(string(data)), System: runner.ExecSystem{}}
	result, err := r.Handle(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "operation refused or uncertain:", err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(1)
	}
}
