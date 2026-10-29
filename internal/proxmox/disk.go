package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
)

var sizeRE = regexp.MustCompile(`(?:^|,)size=([0-9]+(?:\.[0-9]+)?)([KMGT]?)(?:,|$)`)
var diskKeyRE = regexp.MustCompile(`^(?:scsi[0-9]+|virtio[0-9]+|sata[0-9]+|ide[0-9]+)$`)

func DiskGiB(config string) (float64, error) {
	m := sizeRE.FindStringSubmatch(config)
	if m == nil {
		return 0, errors.New("disk size not present in configuration")
	}
	n, e := strconv.ParseFloat(m[1], 64)
	if e != nil || n <= 0 {
		return 0, errors.New("invalid disk size")
	}
	switch m[2] {
	case "K":
		return n / (1024 * 1024), nil
	case "M":
		return n / 1024, nil
	case "G", "":
		return n, nil
	case "T":
		return n * 1024, nil
	default:
		return 0, errors.New("unsupported disk unit")
	}
}
func (c *Client) GrowDisk(ctx context.Context, g Guest, key string, desired string) error {
	if g.Kind == "lxc" && key != "rootfs" {
		return errors.New("only LXC rootfs can be resized")
	}
	if g.Kind == "vm" && !diskKeyRE.MatchString(key) {
		return errors.New("VM resize requires explicit supported disk key")
	}
	cfg, ok := g.Disks[key]
	if !ok {
		return fmt.Errorf("disk %q not present; refusing ambiguous resize", key)
	}
	existing, e := DiskGiB(cfg)
	if e != nil {
		return e
	}
	target, e := strconv.ParseInt(desired, 10, 64)
	if e != nil || target < 1 {
		return errors.New("disk_size must be an integer number of GiB")
	}
	if float64(target) < existing {
		return fmt.Errorf("disk shrink refused (%.2f GiB to %d GiB)", existing, target)
	}
	if float64(target) == existing {
		return nil
	}
	kind := g.Kind
	if kind == "vm" {
		kind = "qemu"
	}
	path := fmt.Sprintf("/nodes/%s/%s/%d/resize", url.PathEscape(g.Node), kind, g.VMID)
	params := url.Values{"disk": {key}, "size": {fmt.Sprintf("%dG", target)}}
	if g.Digest != "" {
		params.Set("digest", g.Digest)
	}
	task, e := c.postConfig(ctx, path, params)
	if e != nil {
		return e
	}
	if task != "" {
		return c.WaitTask(ctx, g.Node, task)
	}
	return nil
}
