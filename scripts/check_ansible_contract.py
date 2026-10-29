#!/usr/bin/env python3
"""Read-only contract drift check against the Ansible feature branch.

Only source text is parsed; the external Python file is never imported/executed.
"""
import argparse
import ast
import re
import urllib.request
from pathlib import Path

ANSIBLE_URL = (
    "https://raw.githubusercontent.com/mwyau/"
    "ansible-proxmox-community-scripts/{ref}/plugins/module_utils/deployment.py"
)

def ansible_maps(text: str) -> dict[str, dict[str, str]]:
    module = ast.parse(text)
    result = {}
    for stmt in module.body:
        if isinstance(stmt, ast.Assign):
            for target in stmt.targets:
                if isinstance(target, ast.Name) and target.id in ("LXC_FIELDS", "VM_FIELDS"):
                    result[target.id] = ast.literal_eval(stmt.value)
    if set(result) != {"LXC_FIELDS", "VM_FIELDS"}:
        raise ValueError("Ansible field maps are unavailable")
    return result

def go_maps(text: str) -> dict[str, dict[str, str]]:
    result = {}
    for go_name, ansible_name in (("LXCFields", "LXC_FIELDS"), ("VMFields", "VM_FIELDS")):
        match = re.search(r"var\s+"+go_name+r"\s*=\s*map\[string\]string\s*\{(.*?)\n\}", text, re.S)
        if match is None:
            raise ValueError(f"Go field map {go_name} unavailable")
        result[ansible_name] = dict(re.findall(r'"([^"]+)"\s*:\s*"([^"]+)"', match.group(1)))
    return result

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--ansible-ref", default="feat/collection-foundation")
    parser.add_argument("--ansible-file", type=Path, help="Offline fixture instead of network request")
    args = parser.parse_args()
    if args.ansible_file:
        source = args.ansible_file.read_text()
    else:
        url=ANSIBLE_URL.format(ref=args.ansible_ref)
        request=urllib.request.Request(url,headers={"User-Agent":"terraform-community-scripts-contract-check"})
        with urllib.request.urlopen(request,timeout=20) as response:
            source=response.read(1_000_001).decode("utf-8")
        if len(source)>1_000_000:
            raise ValueError("Unexpected large Ansible source")
    desired=ansible_maps(source)
    current=go_maps(Path("internal/contract/contract.go").read_text())
    if desired!=current:
        for key in sorted(desired):
            missing={k:v for k,v in desired[key].items() if current.get(key,{}).get(k)!=v}
            extra={k:v for k,v in current.get(key,{}).items() if k not in desired[key]}
            print(f"{key}: missing/changed={missing}; unexpected={extra}")
        raise SystemExit("Go and Ansible normalized field maps differ")
    print("Normalized mappings match Ansible:", len(current["LXC_FIELDS"]), "LXC and",
          len(current["VM_FIELDS"]), "VM fields")

if __name__=="__main__":
    main()
