// Package transport is an SSH transport to the audited PVE host runner.
// SSH host-key verification and noninteractive authentication are required.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/runner"
)

var targetRE = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_.-]*@)?[A-Za-z0-9][A-Za-z0-9.-]*$`)

type SSH struct {
	Target  string
	Command string
}

func NewSSH(target string) (*SSH, error) {
	if target == "" {
		return nil, nil
	}
	if !targetRE.MatchString(target) {
		return nil, errors.New("SSH target must be user@hostname or hostname without shell metacharacters")
	}
	return &SSH{Target: target, Command: "ssh"}, nil
}
func (s *SSH) Run(ctx context.Context, r runner.Request) (runner.Result, error) {
	if s == nil {
		return runner.Result{}, errors.New("ssh_target is required for host-side operations")
	}
	b, e := json.Marshal(r)
	if e != nil {
		return runner.Result{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Command, "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=15", "--", s.Target, "sudo -n /usr/local/sbin/communityscripts-runner execute")
	cmd.Stdin = bytes.NewReader(b)
	// Do not record stderr: upstream installers may print credentials.
	out, e := cmd.Output()
	if e != nil {
		return runner.Result{}, fmt.Errorf("host operation failed or uncertain; inspect durable journal before retry (SSH status: %v)", e)
	}
	if len(out) > 65536 {
		return runner.Result{}, errors.New("oversized runner response")
	}
	var result runner.Result
	if e = json.Unmarshal(out, &result); e != nil {
		return runner.Result{}, errors.New("invalid host runner response; inspect journal")
	}
	return result, nil
}
