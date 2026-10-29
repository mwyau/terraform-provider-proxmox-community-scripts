#!/usr/bin/env python3
"""Fail closed when the local Go Terraform contract diverges from Ansible.

Canonical artifact: ansible-proxmox-community-scripts/contracts/guest.v1.json.
Read-only fetch. No remote code is imported or executed.
"""
import argparse
import ast
import json
import re
import urllib.request
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
ANSIBLE_ROOT="https://raw.githubusercontent.com/mwyau/ansible-proxmox-community-scripts/{ref}"
def retrieve(ref,path):
    if not re.fullmatch(r"[A-Za-z0-9._/-]+",ref) or ".." in ref:
        raise ValueError("invalid Ansible ref")
    uri=ANSIBLE_ROOT.format(ref=ref)+"/"+path
    req=urllib.request.Request(uri,headers={"User-Agent":"communityscripts-contract-ci"})
    with urllib.request.urlopen(req,timeout=20) as resp:
        data=resp.read(1_000_001)
    if len(data)>1_000_000:
        raise ValueError("source larger than 1 MB")
    return data.decode("utf-8")
def python_maps(source):
    tree=ast.parse(source)
    maps={}
    for node in tree.body:
        if isinstance(node,ast.Assign):
            for name in node.targets:
                if isinstance(name,ast.Name) and name.id in ("LXC_FIELDS","VM_FIELDS"):
                    maps[name.id]=ast.literal_eval(node.value)
    if set(maps)!={"LXC_FIELDS","VM_FIELDS"}:
        raise ValueError("missing Ansible field mappings")
    return maps
def go_maps(source):
    mappings={}
    for go,key in (("LXCFields","lxc_fields"),("VMFields","vm_fields")):
        match=re.search(r"var\s+"+go+r"\s*=\s*map\[string\]string\s*\{(.*?)\n\}",source,re.S)
        if not match:raise ValueError("missing Go map "+go)
        mappings[key]=dict(re.findall(r'"([^"]+)"\s*:\s*"([^"]+)"',match.group(1)))
    return mappings
def exposed_keys(source):
    return {m[1] for m in re.finditer(r'^([a-z_][a-z0-9_]*):',source,re.M)}
def schema_keys(source):
    return set(re.findall(r'^\s*"([a-z_][a-z0-9_]*)":\s+schema\.',source,re.M))
def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--ansible-ref",default=(ROOT/"contracts/ansible.sha").read_text().strip(),help="Immutable Ansible commit SHA; override only for development audits")
    a=ap.parse_args()
    if not re.fullmatch(r"[a-f0-9]{40}",a.ansible_ref):
        raise SystemExit("Contract validation requires an immutable 40-character Ansible commit")
    canonical=json.loads((ROOT/"contracts/guest.v1.json").read_text())
    upstream=json.loads(retrieve(a.ansible_ref,"contracts/guest.v1.json"))
    if upstream!=canonical:
        raise SystemExit("Shared contract differs from Ansible. Update both repos in coordinated PRs.")
    py=python_maps(retrieve(a.ansible_ref,"plugins/module_utils/deployment.py"))
    actual=go_maps((ROOT/"internal/contract/contract.go").read_text())
    if (py["LXC_FIELDS"]!=canonical["lxc_fields"] or
        py["VM_FIELDS"]!=canonical["vm_fields"] or
        actual["lxc_fields"]!=canonical["lxc_fields"] or
        actual["vm_fields"]!=canonical["vm_fields"]):
        raise SystemExit("Python or Go mapping differs from normative versioned contract")
    provider=(ROOT/"internal/provider/provider.go").read_text()
    resource=(ROOT/"internal/provider/guest_resource.go").read_text()
    go_keys=schema_keys(provider)|schema_keys(resource)
    expected=set(canonical["common_inputs"])
    if not expected.issubset(go_keys):
        raise SystemExit("Terraform schema missing canonical inputs: "+str(sorted(expected-go_keys)))
    if re.search(r'^\s*"(?:endpoint|insecure|backup_storage)":\s+schema\.',provider+"\n"+resource,re.M):
        raise SystemExit("Legacy Terraform alias was reintroduced")
    ansible_defaults=retrieve(a.ansible_ref,"roles/application/defaults/main.yml")
    ansible_keys=exposed_keys(ansible_defaults)
    if not expected.issubset(ansible_keys):
        raise SystemExit("Ansible facade missing inputs: "+str(sorted(expected-ansible_keys)))
    if not {"ctid","vmid"}.issubset(ansible_keys):
        raise SystemExit("Missing required kind-specific CTID/VMID input in Ansible")
    role=retrieve(a.ansible_ref,"roles/application/tasks/main.yml")
    if "kind == 'lxc' and ctid is not none and vmid is none" not in role or "kind == 'vm' and vmid is not none and ctid is none" not in role:
        raise SystemExit("Ansible must enforce mutually exclusive CTID and VMID")
    if 'm.CTID.IsNull()' not in resource or 'm.VMID.IsNull()' not in resource:
        raise SystemExit("Terraform must enforce mutually exclusive CTID and VMID")
    print("Shared v1 contract parity: 34 LXC, 14 VM; typed CTID/VMID inputs; no deprecated aliases.")
if __name__=="__main__":
    main()
