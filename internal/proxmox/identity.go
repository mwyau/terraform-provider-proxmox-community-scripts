package proxmox

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var scriptURL = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?community-scripts\.org/scripts/([a-z0-9][a-z0-9-]*)`)
var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// DetectScript accepts strong API metadata only; this is not cryptographic proof.
// LXC /usr/bin/update evidence will be added with a guarded host read.
func DetectScript(g Guest) (string, error) {
	found := map[string]bool{}
	for _, m := range scriptURL.FindAllStringSubmatch(g.Description, -1) {
		found[strings.ToLower(m[1])] = true
	}
	if g.Kind == "vm" {
		tags := map[string]bool{}
		for _, s := range strings.Split(g.Tags, ";") {
			tags[strings.ToLower(s)] = true
		}
		if tags["community-script"] && tags["app-deployer"] {
			for s := range tags {
				if strings.HasPrefix(s, "script-") && slug.MatchString(strings.TrimPrefix(s, "script-")) {
					found[strings.TrimPrefix(s, "script-")] = true
				}
			}
		}
	}
	if len(found) == 0 {
		return "", errors.New("script identity unknown: no strong Community Scripts metadata")
	}
	if len(found) > 1 {
		return "", errors.New("conflicting Community Scripts identity evidence")
	}
	for s := range found {
		return s, nil
	}
	return "", errors.New("unreachable identity state")
}
func ValidateExpected(g Guest, kind, script string) error {
	if g.Kind != kind {
		return fmt.Errorf("VMID %d is %s, not %s", g.VMID, g.Kind, kind)
	}
	found, e := DetectScript(g)
	if e != nil {
		return e
	}
	if found != script {
		return fmt.Errorf("VMID %d belongs to %q, not %q", g.VMID, found, script)
	}
	return nil
}
