# SCOPE.md — project-latency

## Overview
Real-time latency visualization tool showing measured RTT between all Linode compute regions. Full-mesh NATS cluster with one node per region, Prometheus for time-series storage, and a retro-styled D3.js map frontend served through Akamai CDN.

## Tier Classification
**Tier 2** — Reusable demo/reference asset

## Domains
- `latency.connected-cloud.io` — origin (hub)
- `latency-demo.connected-cloud.io` — CDN-fronted demo URL
- `{short}.latency.connected-cloud.io` — per-region ping endpoints

## Goals
1. Measure and visualize RTT between all Linode regions using NATS full-mesh cluster routing
2. Measure client-to-region latency from the browser
3. Suggest nearest Linode regions by geographic distance + measured latency
4. Recommend AZ-style region pairs based on latency and fault isolation criteria
5. Present all data on an interactive flat map with continent/country/region filtering
6. Serve through Akamai CDN with DS2 logging to ClickHouse

## Architecture Summary
- **Probe Agents**: 45 Go binaries (34 core Nanodes + 11 distributed edge `g6-dedicated-edge-2`), each running an embedded NATS cluster member
- **Hub Server**: `g6-standard-2` in `us-ord` — NATS cluster seed, Prometheus, REST/WebSocket API, static frontend
- **Frontend**: D3.js retro-styled flat map, vanilla JS, embedded in Go binary via `go:embed`
- **NATS Topology**: Full mesh cluster with promiscuous discovery, RTT from `/routez` monitoring
- **Storage**: Prometheus TSDB on hub, PromQL queries for API
- **CDN**: Akamai Ion (SPM) property with DS2 stream to ClickHouse webhook
- **Auth**: Query-string token auth with client-side auth gate

## Non-Goals
1. HA/failover for the hub — single instance is acceptable for a demo tool
2. Production monitoring — this is a visualization/demo tool, not an alerting system
3. Kubernetes for agents — bare VMs with systemd are simpler and cheaper
4. mTLS for NATS cluster — plaintext with firewall restrictions is acceptable for demo
5. es-mad (Madrid) — region currently restricted for new instance creation

## Exit Criteria
- [x] All agents deployed and reporting latency to hub (45 of 46 — es-mad excluded)
- [x] Map visualization renders all regions and latency arcs with filters
- [ ] Client-to-region latency measurement works from browser (pending FW exception)
- [x] Region finder returns correct results with distance + latency
- [x] AZ-pair selector recommends appropriate pairs with rationale
- [x] Terraform deploys and destroys full stack cleanly
- [x] Akamai CDN property live with token auth
- [x] DS2 stream active and delivering to ClickHouse

## Cost
- 34 core agents (Nanode $5/mo) = $170/mo
- 11 distributed agents (g6-dedicated-edge-2 $43/mo) = $473/mo
- 1 hub (g6-standard-2) = $24/mo
- **Total: ~$667/mo** — can `terraform destroy` when not demoing
