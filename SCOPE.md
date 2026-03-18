# SCOPE.md — project-latency

## Overview
Interactive 3D globe visualizer showing real-time inter-region latency between all Linode regions, measured via NATS request/reply. Includes client-to-region latency measurement, nearest region finder, and AZ-pair selection tools.

## Tier Classification
**Tier 2** — Reusable demo/reference asset

## Domain
`latency.connected-cloud.io`

## Goals
1. Measure and visualize RTT between all 46 Linode regions (1,035 unique pairs) using NATS
2. Measure client-to-region latency from the browser
3. Suggest nearest Linode regions for a given client IP (GeoIP + live measurement)
4. Recommend AZ-style region pairs based on latency and fault isolation criteria
5. Present all data on a spinning 3D globe with interactive controls

## Architecture Summary
- **Probe Agents**: 46 Go binaries on Nanodes (one per region), embedded NATS leaf nodes
- **Hub Server**: Single `g6-standard-2` in `us-ord` — NATS core, REST/WebSocket API, static frontend
- **Frontend**: Globe.gl (Three.js), vanilla JS, Vite build
- **NATS Topology**: Hub-and-spoke with leaf nodes
- **Measurement Cadence**: Full 46-region sweep every 15 minutes, on-demand for targeted pairs

## Non-Goals
1. HA/failover for the hub — single instance is acceptable for a demo tool
2. Persistent time-series storage (Prometheus, InfluxDB) — in-memory + SQLite is sufficient
3. Production monitoring — this is a visualization/demo tool, not an alerting system
4. Kubernetes for agents — bare Nanodes with systemd are simpler and cheaper
5. mTLS for NATS in v1 — user/pass auth with firewall restrictions is acceptable
6. Akamai CDN fronting in v1 — direct-to-origin is fine for demo use

## Exit Criteria
- [ ] All 46 agents deployed and reporting latency to hub
- [ ] Globe visualization renders all regions and latency arcs
- [ ] Client-to-region latency measurement works from browser
- [ ] Nearest region finder returns correct results for test IPs
- [ ] AZ-pair selector recommends appropriate pairs with rationale
- [ ] Terraform deploys and destroys full stack cleanly

## Cost
~$254/mo (46 Nanodes + 1 Standard-2) — can `terraform destroy` when not demoing
