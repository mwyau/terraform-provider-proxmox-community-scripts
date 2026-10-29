// Package recovery defines stable identifiers and fail-closed recovery guidance.
package recovery

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type ID struct {
	Kind string
	VMID int64
}

func (id ID) String() string { return id.Kind + "/" + strconv.FormatInt(id.VMID, 10) }
func ParseID(s string) (ID, error) {
	p := strings.Split(s, "/")
	if len(p) != 2 || (p[0] != "lxc" && p[0] != "vm") || p[1] == "" {
		return ID{}, errors.New("import ID must be lxc/<vmid> or vm/<vmid>")
	}
	n, e := strconv.ParseInt(p[1], 10, 64)
	if e != nil || n < 100 || n > 999999999 || strconv.FormatInt(n, 10) != p[1] {
		return ID{}, errors.New("VMID must be canonical decimal in 100..999999999")
	}
	return ID{Kind: p[0], VMID: n}, nil
}
func RecoveryHint(id ID) string {
	return fmt.Sprintf("Do not re-run the installer or delete the guest automatically. Inspect VMID %d, host-side execution journal and application provenance, then import the verified guest using ID %s. Repair incomplete guests manually.", id.VMID, id.String())
}
