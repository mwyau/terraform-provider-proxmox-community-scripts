// Package compat performs bounded, read-only inspection of pinned Community Scripts
// source during Terraform planning. It never runs shell scripts.
// Bulk recipe reports belong to proxmox-community-scripts-iac-common; this
// independent per-guest check must fail closed regardless of cached status.
package compat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mwyau/terraform-provider-proxmox-community-scripts/internal/contract"
)

const DefaultBase = "https://raw.githubusercontent.com/community-scripts/"

var sha = regexp.MustCompile(`^[0-9a-f]{40}$`)
var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var retired = regexp.MustCompile(`(?im)(?:script\s+(?:is\s+no\s+longer\s+available|was\s+removed)|has\s+been\s+renamed\s+to)`)
var nonzeroExit = regexp.MustCompile(`(?m)^\s*exit\s+[1-9][0-9]*\s*(?:#.*)?$`)
var prompt = regexp.MustCompile(`(?m)(?:\bmsg_menu\b|\bwhiptail\b|\bdialog\b|\bread\s+-[a-zA-Z]*p\b)`)

type Inspector struct {
	HTTP  *http.Client
	Base  string
	Token string
}
type Report struct {
	ScriptPath string
	ScriptsRef string
	CoreRef    string
	Notes      []string
}

func (c Inspector) fetch(ctx context.Context, repo, rev, path string) (string, error) {
	if !sha.MatchString(rev) {
		return "", errors.New("source revision must be a full immutable SHA")
	}
	base := c.Base
	if base == "" {
		base = DefaultBase
	}
	// Base is fixed by provider code; only tests override it. Path is assembled
	// from validated kind/slug and fixed Core paths, never from user-provided URLs.
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("invalid source origin")
	}
	target := strings.TrimRight(base, "/") + "/" + repo + "/" + rev + "/" + path
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("User-Agent", "terraform-provider-proxmox-community-scripts/plan")
	if c.Token != "" && strings.HasPrefix(base, "https://raw.githubusercontent.com/") {
		// Raw content does not require authentication for public repositories.
		// Do not attach private credentials to unexpected hosts or redirects.
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	res, e := client.Do(req)
	if e != nil {
		return "", fmt.Errorf("source unavailable: %w", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("%s at %s returned HTTP %d", path, rev, res.StatusCode)
	}
	body, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if e != nil {
		return "", fmt.Errorf("cannot read pinned source: %w", e)
	}
	if len(body) > 2<<20 {
		return "", errors.New("pinned source exceeds 2 MiB")
	}
	return string(body), nil
}
func (c Inspector) Verify(ctx context.Context, kind, script, scriptsRef, coreRef string, settings map[string]string) (Report, error) {
	if !sha.MatchString(scriptsRef) || !sha.MatchString(coreRef) {
		return Report{}, errors.New("both pinned full scripts_ref and core_ref are required")
	}
	if !slug.MatchString(script) {
		return Report{}, errors.New("invalid script slug")
	}
	if e := contract.ValidateSettings(kind, settings); e != nil {
		return Report{}, e
	}
	directory := "ct"
	corePath := "ui/defaults.func"
	if kind == "vm" {
		directory = "vm"
		corePath = "pve/vm-core.func"
	} else if kind != "lxc" {
		return Report{}, errors.New("invalid kind")
	}
	path := directory + "/" + script + ".sh"
	scriptText, e := c.fetch(ctx, "ProxmoxVE", scriptsRef, path)
	if e != nil {
		return Report{}, e
	}
	lifecycle := "build_container"
	if kind == "vm" {
		lifecycle = "vm_start_script"
	}
	if !strings.Contains(scriptText, lifecycle) {
		if retired.MatchString(scriptText) && nonzeroExit.MatchString(scriptText) {
			return Report{}, fmt.Errorf("script %s appears to be a retired/renamed stub", path)
		}
		return Report{}, fmt.Errorf("script %s lacks required %s entrypoint; custom scripts need separate review", path, lifecycle)
	}
	if kind == "vm" && !strings.Contains(scriptText, "pve/vm-core.func") {
		return Report{}, errors.New("VM script does not use the shared vm-core.func entrypoint")
	}
	if kind == "lxc" && !strings.Contains(scriptText, "core/build.func") {
		return Report{}, errors.New("LXC script does not use the shared core/build.func entrypoint")
	}
	coreText, e := c.fetch(ctx, "core", coreRef, corePath)
	if e != nil {
		return Report{}, e
	}
	if kind == "vm" && (!strings.Contains(coreText, "vm_apply_env_overrides()") || !strings.Contains(coreText, "VM_UNATTENDED")) {
		return Report{}, errors.New("pinned shared VM Core lacks unattended override contract")
	}
	fields, _ := contract.Fields(kind)
	for key := range settings {
		coreName := fields[key]
		if !strings.Contains(coreText, coreName) {
			return Report{}, fmt.Errorf("pinned Core %s does not contain expected override %s for setting %s", corePath, coreName, key)
		}
	}
	result := Report{ScriptPath: path, ScriptsRef: scriptsRef, CoreRef: coreRef}
	if prompt.MatchString(scriptText) {
		result.Notes = append(result.Notes, "source contains potential interactive prompts; static inspection cannot prove unattended installation")
	}
	if kind == "vm" {
		for key := range settings {
			variable := map[string]string{"cores": "CORE_COUNT", "memory": "RAM_SIZE", "disk_size": "DISK_SIZE", "hostname": "HN", "bridge": "BRG", "vlan_id": "VLAN", "datastore_id": "STORAGE", "mac_address": "MAC", "mtu": "MTU", "cpu_type": "CPU_TYPE", "machine": "MACHINE", "started": "vm_start_vm", "disk_cache": "DISK_CACHE", "timezone": "vm_timezone_default"}[key]
			if variable != "" && !strings.Contains(scriptText, variable) {
				result.Notes = append(result.Notes, fmt.Sprintf("VM field %s lacks per-script usage evidence (%s); shared Core support is not sufficient", key, variable))
			}
		}
	}
	result.Notes = append(result.Notes, "static source inspection only; installer not executed, app prompts and external dependencies not certified")
	return result, nil
}
