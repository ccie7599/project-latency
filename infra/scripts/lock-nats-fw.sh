#!/bin/bash
# Lock down NATS cluster port 6222 to the current mesh member IPs across all 4 firewalls.
# Run this any time agents are added/removed or LKE nodes are recycled.
#
# Reads tokens from:
#   - ~/project-landing-zone/presales-landing-zone/.linode-token  (presales account)
#   - ~/.config/linode-cli                                         (old/demo account)

set -euo pipefail

python3 << 'PYEOF'
import requests, json, os, sys

pt = open(os.path.expanduser('~/project-landing-zone/presales-landing-zone/.linode-token')).read().strip().split('\n')[-1]
ot = None
with open(os.path.expanduser('~/.config/linode-cli')) as f:
    for line in f:
        if line.strip().startswith('token'):
            ot = line.split('=')[1].strip()
            break

cluster_ips = set()

# Presales VMs + LKE worker nodes
resp = requests.get('https://api.linode.com/v4/linode/instances',
    headers={'Authorization': f'Bearer {pt}'}, params={'page_size': 500})
for inst in resp.json().get('data', []):
    if 'project:latency' in inst.get('tags', []) or inst['label'].startswith('lke') or inst['label'] == 'latency-hub':
        for ip in inst.get('ipv4', []):
            if not ip.startswith('192.168.'):
                cluster_ips.add(ip)

# LKE nodes scanned via cluster API (ensures we catch all worker nodes)
clusters = requests.get('https://api.linode.com/v4/lke/clusters',
    headers={'Authorization': f'Bearer {pt}'}).json().get('data', [])
for c in clusters:
    if c['label'].startswith('latency-'):
        pools = requests.get(f'https://api.linode.com/v4/lke/clusters/{c["id"]}/pools',
            headers={'Authorization': f'Bearer {pt}'}).json().get('data', [])
        for p in pools:
            for node in p.get('nodes', []):
                nid = node.get('instance_id')
                if nid:
                    ni = requests.get(f'https://api.linode.com/v4/linode/instances/{nid}',
                        headers={'Authorization': f'Bearer {pt}'}).json()
                    for ip in ni.get('ipv4', []):
                        if not ip.startswith('192.168.'):
                            cluster_ips.add(ip)

# Hub node external IP (fixed, in case it's not picked up above)
cluster_ips.add('172.238.169.30')

# Old account remaining agents
if ot:
    resp_old = requests.get('https://api.linode.com/v4/linode/instances',
        headers={'Authorization': f'Bearer {ot}'}, params={'page_size': 500})
    for inst in resp_old.json().get('data', []):
        if 'project:latency' in inst.get('tags', []):
            for ip in inst.get('ipv4', []):
                if not ip.startswith('192.168.'):
                    cluster_ips.add(ip)

cluster_cidrs = sorted(f"{ip}/32" for ip in cluster_ips)
print(f"Mesh members: {len(cluster_cidrs)} IPs")

# Apply to presales firewalls
headers = {'Authorization': f'Bearer {pt}', 'Content-Type': 'application/json'}
for fw_id, fw_name in [(3895879, 'presales-landing-zone'), (4093969, 'latency-agents-fw'), (4094050, 'latency-vm-agents-fw')]:
    resp = requests.get(f'https://api.linode.com/v4/networking/firewalls/{fw_id}/rules', headers=headers)
    rules = resp.json()
    for r in rules.get('inbound', []):
        if r.get('ports') == '6222':
            r['addresses']['ipv4'] = cluster_cidrs
            r['addresses'].pop('ipv6', None)
    resp2 = requests.put(f'https://api.linode.com/v4/networking/firewalls/{fw_id}/rules',
        headers=headers, json=rules)
    print(f"  presales/{fw_name}: {resp2.status_code}")

# Old account firewall
if ot:
    headers_old = {'Authorization': f'Bearer {ot}', 'Content-Type': 'application/json'}
    resp = requests.get('https://api.linode.com/v4/networking/firewalls/3986525/rules', headers=headers_old)
    rules = resp.json()
    for r in rules.get('inbound', []):
        if r.get('ports') == '6222':
            r['addresses']['ipv4'] = cluster_cidrs
            r['addresses'].pop('ipv6', None)
    resp2 = requests.put('https://api.linode.com/v4/networking/firewalls/3986525/rules',
        headers=headers_old, json=rules)
    print(f"  old-account/latency-agents-fw: {resp2.status_code}")
PYEOF
