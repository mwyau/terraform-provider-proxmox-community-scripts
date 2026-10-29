# Terraform provider for Proxmox Community Scripts

Experimental native Go provider, intended to complement [the Ansible Community Scripts collection](https://github.com/mwyau/ansible-proxmox-community-scripts) and [bpg/proxmox](https://github.com/bpg/terraform-provider-proxmox).

**Development stage: inspection/import only. No guest creation, update, or deletion is enabled.** The `communityscripts_guest` resource can import an existing Community Scripts LXC/VM and refresh its Proxmox state after checking identity. A Create or Update returns an error without changing the host. Destroy removes only Terraform state, retaining the real guest.

## Usage

See [examples/import.tf](examples/import.tf). Provider type: `communityscripts`; proposed Registry address: `mwyau/proxmox-community-scripts` (**not published**). Import an existing positively identified guest:

```sh
terraform import communityscripts_guest.adguard lxc/120
terraform plan
```

Import IDs are `lxc/<vmid>` or `vm/<vmid>`; the provider finds its current node using the cluster API. A guest whose provenance is unknown or conflicting cannot be imported. For now, only the source URL in the Proxmox description and strong VM Core tags are recognized; host-assisted LXC updater inspection is not implemented. These attributes are evidence, not cryptographic attestation.

The Proxmox API client uses HTTPS token authentication and verifies TLS by default. The `insecure` setting is only for isolated testing. Use a token with the minimum permissions needed to read cluster resources, guest config, and status. Do not commit credentials or Terraform state.

## Interrupted creation

Terraform state alone cannot reliably prevent re-execution after the provider dies between guest allocation and state persistence. **Create is intentionally disabled** until the durable host-side journal, cross-node exclusion, and recovery protocol in [docs/INTERRUPTED_CREATION.md](docs/INTERRUPTED_CREATION.md) is implemented and tested. No automatic retry, guest deletion, or adoption is permitted on uncertain installer results.

## Development

Go 1.23+ with HashiCorp Terraform Plugin Framework v1.15.1:

```sh
go mod tidy
gofmt -w main.go internal/
go vet ./...
go test -race ./...
go build ./...
```

CI uses mock Proxmox HTTPS responses; **no live Proxmox integration tests have passed**. The provider is not yet released or published to the Terraform Registry. Do not put the same guest under competing managed resource blocks from this provider and bpg/proxmox.

Next: durable install coordinator and journal, fault-injection tests, and a single guarded LXC Create against disposable Proxmox. VM creation and updates follow only after separate live validation.

MIT licensed. Not affiliated with Proxmox or the Community Scripts maintainers.
