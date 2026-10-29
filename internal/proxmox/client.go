// Package proxmox provides a minimal read-only Proxmox API client.
package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Guest struct {
	VMID                                  int64
	Node, Kind, Description, Tags, Status string
}
type Client struct {
	base, token string
	http        *http.Client
}

func NewClient(endpoint, tokenID, tokenSecret string, insecure bool) (*Client, error) {
	if endpoint == "" || tokenID == "" || tokenSecret == "" {
		return nil, errors.New("endpoint and API token are required")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("endpoint must be a Proxmox HTTPS origin")
	}
	host := u.Hostname()
	if u.Port() == "" {
		u.Host = host + ":8006"
		if strings.Contains(host, ":") {
			u.Host = "[" + host + "]:8006"
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} // explicit test-only opt-in
	return &Client{base: strings.TrimSuffix(u.String(), "/"), token: "PVEAPIToken=" + tokenID + "=" + tokenSecret, http: &http.Client{Transport: tr, Timeout: 20 * time.Second}}, nil
}
func (c *Client) get(ctx context.Context, path string, out any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api2/json"+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", c.token)
	res, e := c.http.Do(req)
	if e != nil {
		return fmt.Errorf("Proxmox request failed: %w", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("Proxmox API returned HTTP %d for %s", res.StatusCode, path)
	}
	var data struct {
		Data json.RawMessage `json:"data"`
	}
	if e = json.NewDecoder(http.MaxBytesReader(nil, res.Body, 2<<20)).Decode(&data); e != nil {
		return fmt.Errorf("invalid Proxmox response: %w", e)
	}
	if len(data.Data) == 0 || string(data.Data) == "null" {
		return errors.New("Proxmox response missing data")
	}
	if e = json.Unmarshal(data.Data, out); e != nil {
		return fmt.Errorf("invalid Proxmox data: %w", e)
	}
	return nil
}
func (c *Client) FindGuest(ctx context.Context, vmid int64) (Guest, bool, error) {
	if vmid < 100 || vmid > 999999999 {
		return Guest{}, false, errors.New("VMID out of range")
	}
	var entries []struct {
		VMID int64  `json:"vmid"`
		Node string `json:"node"`
		Type string `json:"type"`
	}
	if e := c.get(ctx, "/cluster/resources?type=vm", &entries); e != nil {
		return Guest{}, false, e
	}
	var g Guest
	found := false
	for _, x := range entries {
		if x.VMID != vmid {
			continue
		}
		if found {
			return Guest{}, false, fmt.Errorf("duplicate VMID %d", vmid)
		}
		kind := x.Type
		if kind == "qemu" {
			kind = "vm"
		}
		if x.Node == "" || (kind != "lxc" && kind != "vm") {
			return Guest{}, false, errors.New("invalid guest node/type")
		}
		g = Guest{VMID: vmid, Node: x.Node, Kind: kind}
		found = true
	}
	if !found {
		return Guest{}, false, nil
	}
	kind := g.Kind
	if kind == "vm" {
		kind = "qemu"
	}
	path := "/nodes/" + url.PathEscape(g.Node) + "/" + kind + "/" + strconv.FormatInt(vmid, 10)
	var cfg struct {
		Description string `json:"description"`
		Tags        string `json:"tags"`
	}
	if e := c.get(ctx, path+"/config", &cfg); e != nil {
		return Guest{}, false, e
	}
	var state struct {
		Status string `json:"status"`
	}
	if e := c.get(ctx, path+"/status/current", &state); e != nil {
		return Guest{}, false, e
	}
	g.Description = cfg.Description
	g.Tags = cfg.Tags
	g.Status = state.Status
	return g, true, nil
}
