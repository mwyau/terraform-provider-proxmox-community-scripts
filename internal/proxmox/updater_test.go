package proxmox

import "testing"

func TestUpdaterIdentity(t *testing.T) {
	for _, c := range []struct {
		script string
		want   string
		bad    bool
	}{
		{"export SCRIPT_SLUG='adguard'\n", "adguard", false},
		{"#!/bin/sh\nwget https://github.com/community-scripts/ProxmoxVE/raw/main/ct/adguard.sh\n", "adguard", false},
		{"export SCRIPT_SLUG=adguard\nexport SCRIPT_SLUG=ubuntu\n", "", true},
		{"UPDATE_SCRIPT_NAME=debian\n", "", true},
	} {
		got, e := DetectUpdaterSlug(c.script)
		if got != c.want || (e != nil) != c.bad {
			t.Fatalf("text %q => %q %v", c.script, got, e)
		}
	}
}
