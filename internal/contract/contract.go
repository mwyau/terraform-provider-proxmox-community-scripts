// Package contract mirrors Ansible's normalized Community Scripts settings.
// The upstream Ansible mapping and this package must pass the cross-repo CI check.
package contract

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

var LXCFields = map[string]string{
	"cores":            "var_cpu",
	"memory":           "var_ram",
	"disk_size":        "var_disk",
	"hostname":         "var_hostname",
	"bridge":           "var_brg",
	"vlan":             "var_vlan",
	"storage":          "var_container_storage",
	"template_storage": "var_template_storage",
	"os":               "var_os",
	"os_version":       "var_version",
	"privileged":       "var_unprivileged",
	"ipv4":             "var_net",
	"gateway":          "var_gateway",
	"ipv6_method":      "var_ipv6_method",
	"mac":              "var_mac",
	"mtu":              "var_mtu",
	"dns":              "var_ns",
	"searchdomain":     "var_searchdomain",
	"timezone":         "var_timezone",
	"tags":             "var_tags",
	"protection":       "var_protection",
	"nesting":          "var_nesting",
	"tun":              "var_tun",
	"fuse":             "var_fuse",
	"keyctl":           "var_keyctl",
	"gpu":              "var_gpu",
	"mknod":            "var_mknod",
	"mount_fs":         "var_mount_fs",
	"apt_cacher":       "var_apt_cacher",
	"apt_cacher_ip":    "var_apt_cacher_ip",
	"inherit_host_ca":  "var_inherit_host_ca",
	"sdn_vnet":         "var_sdn_vnet",
	"http_proxy":       "var_http_proxy",
	"http_no_proxy":    "var_http_no_proxy",
}
var VMFields = map[string]string{
	"cores":      "VM_CORES",
	"memory":     "VM_RAM",
	"disk_size":  "VM_DISK_SIZE",
	"hostname":   "VM_HOSTNAME",
	"bridge":     "VM_BRIDGE",
	"vlan":       "VM_VLAN",
	"storage":    "VM_STORAGE",
	"mac":        "VM_MAC",
	"mtu":        "VM_MTU",
	"cpu_type":   "VM_CPU",
	"machine":    "VM_MACHINE",
	"start":      "VM_START",
	"disk_cache": "VM_DISK_CACHE",
	"timezone":   "VM_TIMEZONE",
}
var envIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var slugName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,62}$`)
var shortName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var tzName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+.-]*(?:/[A-Za-z0-9_+.-]+)*$`)
var osName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var osVersion = regexp.MustCompile(`^[A-Za-z0-9.]+$`)
var fsList = regexp.MustCompile(`^[a-zA-Z0-9_+-]+(?:,[a-zA-Z0-9_+-]+)*$`)
var aptEndpoint = regexp.MustCompile(`^(?:https?://)?[A-Za-z0-9.:\[\]_-]+(?:/[A-Za-z0-9._/+~-]*)?$`)
var reserved = map[string]bool{
	"PATH": true, "HOME": true, "SHELL": true, "ENV": true, "BASH_ENV": true, "BASHOPTS": true, "SHELLOPTS": true,
	"IFS": true, "CDPATH": true, "GLOBIGNORE": true, "PROMPT_COMMAND": true, "TERM": true,
	"mode": true, "MODE": true, "PHS_SILENT": true, "VM_VMID": true, "VM_UNATTENDED": true,
	"var_ctid": true, "var_unattended": true, "VM_CLOUD_INIT": true, "var_ssh": true,
	"var_ssh_authorized_key": true, "VM_SETTINGS_MODE": true, "VM_CREATED": true,
}
var reservedPrefixes = []string{"COMMUNITY_SCRIPTS_", "PHS_", "LD_", "DYLD_", "BASH_FUNC_", "ANSIBLE_", "PYTHON", "GIT_", "CLOUDINIT_"}

func Fields(kind string) (map[string]string, error) {
	switch kind {
	case "lxc":
		return LXCFields, nil
	case "vm":
		return VMFields, nil
	default:
		return nil, fmt.Errorf("unsupported guest kind %q", kind)
	}
}
func normalizedBoolean(s string) (bool, error) {
	switch s {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("boolean must be true/false, yes/no, or 1/0")
	}
}
func numericSetting(k, v string) error {
	if v == "" || strings.Trim(v, "0123456789") != "" {
		return fmt.Errorf("%s must be an unsigned integer", k)
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil {
		return e
	}
	switch k {
	case "cores":
		if n < 1 || n > 128 {
			return errors.New("cores must be 1..128")
		}
	case "memory":
		if n < 256 || n > 1048576 {
			return errors.New("memory must be 256..1048576 MiB")
		}
	case "disk_size":
		if n < 1 || n > 100000 {
			return errors.New("disk_size must be 1..100000 GiB")
		}
	case "vlan":
		if n < 1 || n > 4094 {
			return errors.New("vlan must be 1..4094; omit for untagged")
		}
	case "mtu":
		if n < 576 || n > 65535 {
			return errors.New("mtu must be 576..65535")
		}
	}
	return nil
}

// ValidateSettings validates the normalized public vocabulary, not arbitrary
// upstream script options. App-specific choices belong in ScriptVars.
func ValidateSettings(kind string, values map[string]string) error {
	fields, e := Fields(kind)
	if e != nil {
		return e
	}
	for k, v := range values {
		if _, ok := fields[k]; !ok {
			return fmt.Errorf("unknown normalized %s setting %q", kind, k)
		}
		if len(v) > 4096 || strings.ContainsAny(v, "\x00\r\n") {
			return fmt.Errorf("%s contains forbidden control characters or exceeds size limit", k)
		}
		switch k {
		case "cores", "memory", "disk_size", "vlan", "mtu":
			if e := numericSetting(k, v); e != nil {
				return e
			}
		case "privileged", "start", "nesting", "keyctl", "mknod", "tun", "fuse", "protection", "gpu", "apt_cacher", "inherit_host_ca":
			if _, e := normalizedBoolean(v); e != nil {
				return fmt.Errorf("%s: %w", k, e)
			}
		case "ipv6_method":
			if v != "none" && v != "auto" && v != "dhcp" {
				return errors.New("ipv6_method must be none, auto or dhcp; static IPv6 is unverified")
			}
		case "ipv4":
			if v != "dhcp" {
				if _, e := netip.ParsePrefix(v); e != nil {
					return fmt.Errorf("invalid ipv4 prefix: %w", e)
				}
			}
		case "gateway", "dns":
			if _, e := netip.ParseAddr(v); e != nil {
				return fmt.Errorf("%s must be an IP address", k)
			}
		case "mac":
			if _, e := net.ParseMAC(v); e != nil {
				return fmt.Errorf("invalid MAC: %w", e)
			}
		case "hostname":
			if !slugName.MatchString(v) {
				return errors.New("invalid hostname")
			}
		case "os":
			if !osName.MatchString(v) {
				return errors.New("invalid os")
			}
		case "os_version":
			if !osVersion.MatchString(v) {
				return errors.New("invalid os_version")
			}
		case "timezone":
			if !tzName.MatchString(v) {
				return errors.New("invalid timezone")
			}
		case "mount_fs":
			if !fsList.MatchString(v) {
				return errors.New("invalid mount_fs")
			}
		case "apt_cacher_ip":
			if len(v) > 512 || !aptEndpoint.MatchString(v) {
				return errors.New("invalid apt_cacher_ip")
			}
		case "cpu_type":
			if v != "host" {
				return errors.New("only cpu_type=host is supported by shared Core")
			}
		case "machine":
			if v != "q35" && v != "i440fx" {
				return errors.New("machine must be q35 or i440fx")
			}
		case "disk_cache":
			if v != "none" && v != "writethrough" {
				return errors.New("disk_cache must be none or writethrough")
			}
		case "bridge", "storage", "template_storage", "sdn_vnet":
			if !shortName.MatchString(v) {
				return fmt.Errorf("%s is invalid", k)
			}
		}
	}
	return nil
}
func ScriptVars(values map[string]string) error {
	if len(values) > 64 {
		return errors.New("script_vars may contain at most 64 entries")
	}
	total := 0
	for k, v := range values {
		if len(k) > 128 || !envIdentifier.MatchString(k) || reserved[k] {
			return fmt.Errorf("invalid or protected script_vars key %q", k)
		}
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(k, p) {
				return fmt.Errorf("protected script_vars prefix %q", k)
			}
		}
		for _, fields := range []map[string]string{LXCFields, VMFields} {
			for _, raw := range fields {
				if k == raw {
					return fmt.Errorf("script_vars cannot override normalized Core variable %q", k)
				}
			}
		}
		if strings.ContainsAny(v, "\x00\r\n") {
			return fmt.Errorf("script_vars %q contains forbidden control characters", k)
		}
		total += len(k) + len(v)
		if total > 32768 {
			return errors.New("script_vars exceed 32 KiB")
		}
	}
	return nil
}
func ResolveDefaults(kind string, defaults map[string]map[string]string, settings map[string]string) (map[string]string, error) {
	if _, e := Fields(kind); e != nil {
		return nil, e
	}
	for k, m := range defaults {
		if k != "all" && k != "lxc" && k != "vm" {
			return nil, fmt.Errorf("unknown guest_defaults layer %q", k)
		}
		if e := ValidateSettings(map[bool]string{true: "vm", false: "lxc"}[k == "vm"], m); e != nil && k != "all" {
			return nil, e
		}
	}
	out := map[string]string{}
	for _, layer := range []string{"all", kind} {
		for k, v := range defaults[layer] {
			out[k] = v
		}
	}
	for k, v := range settings {
		out[k] = v
	}
	return out, ValidateSettings(kind, out)
}
func Environment(kind string, settings, vars map[string]string) (map[string]string, error) {
	if e := ValidateSettings(kind, settings); e != nil {
		return nil, e
	}
	if e := ScriptVars(vars); e != nil {
		return nil, e
	}
	fields, _ := Fields(kind)
	env := map[string]string{}
	for k, v := range settings {
		key := fields[k]
		switch k {
		case "privileged":
			b, _ := normalizedBoolean(v)
			if b {
				env[key] = "0"
			} else {
				env[key] = "1"
			}
		case "start", "tun", "fuse", "protection", "gpu", "apt_cacher", "inherit_host_ca":
			b, _ := normalizedBoolean(v)
			if b {
				env[key] = "yes"
			} else {
				env[key] = "no"
			}
		case "nesting", "keyctl", "mknod":
			b, _ := normalizedBoolean(v)
			if b {
				env[key] = "1"
			} else {
				env[key] = "0"
			}
		case "disk_size":
			if kind == "vm" {
				env[key] = v + "G"
			} else {
				env[key] = v
			}
		case "disk_cache":
			env[key] = "cache=" + v + ","
		default:
			env[key] = v
		}
	}
	for k, v := range vars {
		if _, exists := env[k]; exists {
			return nil, fmt.Errorf("script_vars cannot override settings key %q", k)
		}
		env[k] = v
	}
	return env, nil
}
