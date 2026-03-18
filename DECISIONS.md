# DECISIONS.md — project-latency

## ADR-001: NATS Hub-and-Spoke Topology
**Date**: 2026-03-18
**Status**: Superseded by ADR-005

**Decision**: Hub-and-spoke with NATS leaf nodes connecting to a single core server in us-ord.

**Why superseded**: Hub routing inflates RTT measurements. Full mesh cluster gives direct region-to-region latency and eliminates custom probe code entirely.

---

## ADR-002: Globe.gl for 3D Visualization
**Date**: 2026-03-18
**Status**: Superseded by ADR-007

**Decision**: Globe.gl (Three.js) spinning 3D globe.

**Why superseded**: Retro DEFCON-style flat map provides better readability, zoom capability, and a more memorable demo aesthetic.

---

## ADR-003: Bare Nanodes with systemd (No K8s/Docker)
**Date**: 2026-03-18
**Status**: Accepted

**Decision**: Cross-compiled Go binary deployed to bare Nanodes ($5/mo each), managed by systemd, provisioned via Terraform.

**Rationale**: Go binary + embedded NATS server runs with <200MB RAM on a 1GB Nanode. No container orchestration needed.

---

## ADR-004: In-Memory + SQLite for Latency Storage
**Date**: 2026-03-18
**Status**: Superseded by ADR-006

**Decision**: Primary store is in-memory Go map. SQLite for persistence.

**Why superseded**: Prometheus gives us time-series storage, PromQL querying, percentiles, aggregation, and Grafana compatibility — all for free.

---

## ADR-005: Full Mesh NATS Cluster with Promiscuous Discovery
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need to measure latency between 46 Linode regions. ADR-001's hub-and-spoke approach routes all measurements through a single hub, inflating RTT and requiring custom probe code.

**Decision**: Deploy a NATS server in each of the 46 regions, forming a single stretched cluster with full mesh routing. Each node is configured with a seed route to the hub; NATS gossip (promiscuous discovery) handles the rest. NATS natively measures RTT on each cluster route.

**Rationale**:
- Full mesh gives **direct** region-to-region RTT (no hub routing overhead)
- NATS cluster gossip automatically discovers and connects all members — configure one seed, get full mesh
- The NATS `/routez` monitoring endpoint exposes RTT for every route, with `server_name` labels — no custom measurement code needed
- 46 nodes × 45 routes = 1,035 bidirectional connections — well within NATS cluster capacity
- Eliminates all custom probe request/reply code, scheduling, staggering, and aggregation

**NATS Cluster Config** (per node):
```
server_name: <region-id>
cluster {
  name: latency-mesh
  listen: 0.0.0.0:6222
  routes [ nats-route://hub-ip:6222 ]
  # no_advertise defaults to false — promiscuous discovery enabled
}
http_port: 8222  # monitoring (/routez, /varz)
```

**Consequences**: Each Nanode runs a NATS server process (additional ~50MB RAM vs client-only). Port 6222 must be open between all nodes. This is a demo tool so 1,035 cross-region TCP connections are acceptable. If a node can't reach the seed on bootstrap, it waits and retries — once the mesh is established, routes persist even if the seed goes down.

---

## ADR-006: Prometheus for Latency Storage and PromQL for API Queries
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need to store and query time-series latency data. ADR-004's in-memory approach lacks persistence, history, aggregation, and ecosystem integration.

**Decision**: Run Prometheus on the hub VM. The hub Go binary scrapes `/routez` from all NATS cluster members, exposes per-route RTT as Prometheus gauge metrics, and Prometheus scrapes this. Hub API endpoints translate requests into PromQL queries against the local Prometheus instance.

**Data flow**:
```
NATS nodes (/routez)  →  Hub scraper  →  Prometheus gauge metrics  →  Prometheus TSDB
                                                                            ↓
Frontend  ←  Hub REST API  ←  PromQL queries  ←─────────────────────────────┘
```

**Metric exposed**:
```
nats_cluster_route_rtt_seconds{source="us-ord", target="fr-par"} 0.098
```

**Rationale**:
- Prometheus gives time-series history, percentiles (`quantile_over_time`), aggregation (`avg`, `max`), and alerting — all via PromQL
- Standard ecosystem — can plug in Grafana for dashboards with zero additional work
- 1,035 time series at 10s scrape interval is trivially small for Prometheus
- Hub API uses `github.com/prometheus/client_golang/api` to issue PromQL queries — clean separation between storage and presentation
- No custom storage code to maintain

**Prometheus config** (on hub):
```yaml
global:
  scrape_interval: 10s
scrape_configs:
  - job_name: nats-latency
    static_configs:
      - targets: ['localhost:2112']
```

**Consequences**: Adds Prometheus as a dependency on the hub VM (~100MB binary, ~200MB RAM). Queries have ~10s staleness (scrape interval). Retention defaulted to 15 days. Acceptable for a demo tool.

---

## ADR-007: Retro DEFCON-Style Flat Map Visualization
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need a memorable, distinctive visualization for demos. ADR-002's Globe.gl spinning globe is generic and limits readability at scale.

**Decision**: D3.js-based flat world map with a retro green-on-black aesthetic inspired by DEFCON, WarGames, and cold-war era command center displays. Supports zoom/pan.

**Rationale**:
- Flat projection shows all 46 regions simultaneously without rotation
- Zoom/pan lets presenters drill into specific regions during demos
- Green-on-black CRT aesthetic is instantly recognizable and memorable — differentiates from generic dashboards
- D3.js gives full control over rendering, interactions, and projections
- CSS scanline and phosphor glow effects sell the aesthetic without JavaScript overhead

**Visual elements**:
- Black background with green country outlines (Natural Earth projection)
- Lat/lon grid lines in dark green
- Region markers as pulsing green dots
- Latency connections as green arcs with brightness proportional to speed (brighter = lower latency)
- CRT scanline overlay via CSS
- Monospace terminal-style panels and text

**Consequences**: No 3D globe view. D3.js is heavier than Globe.gl for this specific use case but far more flexible. Natural Earth projection requires loading TopoJSON world data (~100KB from CDN).
