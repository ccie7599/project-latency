# Architecture — Linode Global Latency Map

This document describes the full system architecture, data flow, deployment model, and design rationale for the Linode inter-region latency visualizer.

Companion Excalidraw diagrams are in `docs/diagrams/`.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Components](#components)
3. [NATS Topology & Messaging](#nats-topology--messaging)
4. [Latency Measurement](#latency-measurement)
5. [Data Flow](#data-flow)
6. [API Surface](#api-surface)
7. [Frontend & UI](#frontend--ui)
8. [Deployment Model](#deployment-model)
9. [Security Model](#security-model)
10. [Cost Model](#cost-model)

---

## System Overview

**Diagram: `docs/diagrams/01-system-overview.excalidraw`**

The system has three logical components:

| Component | Count | What it does |
|-----------|-------|-------------|
| **Hub** | 1 | Central NATS server, REST/WebSocket API, static frontend host |
| **Probe Agents** | 46 | Lightweight daemons that measure latency to other agents via NATS |
| **Frontend** | 1 (static) | 3D globe visualization served from the hub |

The hub runs in `us-ord` (Chicago). One probe agent runs on a Nanode ($5/mo) in each of the 46 Linode regions worldwide. The frontend is a single-page application embedded in the hub binary and served as static files.

### Why this shape?

The alternative is peer-to-peer measurement (every agent talks to every other agent directly). That requires:
- 1,035 direct TCP connections (46 choose 2)
- Each agent to know the IP of every other agent
- A separate aggregation layer to collect results

Hub-and-spoke is simpler: each agent connects to one place (the hub), the hub routes messages between agents, and the hub is already where the API lives. The trade-off is that the hub is a single point of failure — acceptable for a demo tool.

---

## Components

### Hub (`cmd/hub/`)

**Diagram: `docs/diagrams/02-hub-internals.excalidraw`**

The hub is a single Go binary that runs four subsystems in one process:

```
┌─────────────────────────────────────────────────────────┐
│  Hub Process                                            │
│                                                         │
│  ┌──────────────────┐   ┌───────────────────────────┐   │
│  │ Embedded NATS    │   │ HTTP Server               │   │
│  │ Core Server      │   │                           │   │
│  │                  │   │ GET /api/v1/matrix        │   │
│  │ :4222 (client)   │   │ GET /api/v1/regions       │   │
│  │ :7422 (leaf)     │   │ GET /api/v1/pair/:a/:b    │   │
│  │                  │   │ GET /api/v1/nearest       │   │
│  └────────┬─────────┘   │ GET /api/v1/az-pairs      │   │
│           │              │ GET /api/v1/health        │   │
│  ┌────────▼─────────┐   │ GET /api/v1/trigger-sweep │   │
│  │ Aggregator       │   │ WS  /ws/live              │   │
│  │                  │   │ GET /ping                  │   │
│  │ Subscribes to    │   │ GET / (static frontend)    │   │
│  │ latency.results  │   └───────────────────────────┘   │
│  │ Updates matrix   │                                   │
│  │ Broadcasts WS    │   ┌───────────────────────────┐   │
│  └──────────────────┘   │ Sweep Scheduler           │   │
│                         │                           │   │
│  ┌──────────────────┐   │ Every 15 min: publishes   │   │
│  │ In-Memory Matrix │   │ latency.control.schedule  │   │
│  │                  │   │ with list of 46 region IDs│   │
│  │ Latest: map[][]  │   └───────────────────────────┘   │
│  │ History: 24h     │                                   │
│  └──────────────────┘                                   │
└─────────────────────────────────────────────────────────┘
```

**Why embedded NATS?** Running NATS as a separate process adds deployment complexity (two binaries, two systemd units, coordination). Embedding means one binary to deploy, one process to monitor, one thing to restart. The NATS server library (`github.com/nats-io/nats-server/v2`) supports this natively.

**Why in-memory storage?** The full matrix is 1,035 pairs × ~200 bytes = ~200KB. With 24 hours of 15-minute samples (96 data points per pair), total memory is ~20MB. No database needed.

### Probe Agent (`cmd/agent/`)

Each agent is a ~15MB statically-compiled Go binary running on a Nanode. It does three things:

1. **Responds to probes** — Subscribes to `latency.probe.request.<own-region>` and replies instantly with a timestamp. This is the "target" side of a measurement.

2. **Runs probes** — When told to sweep, sends NATS `Request()` calls to every other region's probe subject. Measures round-trip time. Publishes results.

3. **Serves HTTP /ping** — A simple JSON endpoint for browser-based client-to-region latency measurement.

```
┌────────────────────────────────┐
│  Agent (e.g., region=fr-par)   │
│                                │
│  NATS Connection → hub:7422    │
│  ├─ SUB latency.probe.request.fr-par  (respond to probes)
│  ├─ SUB latency.control.schedule      (trigger sweeps)
│  └─ SUB latency.control.probe.fr-par  (targeted probe)
│                                │
│  HTTP :8080                    │
│  ├─ GET /ping   → {"region":"fr-par","ts":...}
│  └─ GET /health → {"status":"ok","nats_connected":true}
└────────────────────────────────┘
```

### Frontend (`cmd/hub/static/`)

Single HTML file with inline CSS and JavaScript. Uses Globe.gl (a Three.js wrapper purpose-built for data-on-globe rendering). No build step — loads Globe.gl from CDN.

Five views:
1. **Globe** — Spinning 3D globe with latency arcs between regions
2. **Heatmap** — 46×46 canvas matrix, color-coded by RTT
3. **Client Latency** — Measures your browser's RTT to each region
4. **AZ Pair Selector** — Picks region pairs for replication strategies
5. **Region Finder** — Finds nearest Linode regions to a lat/lon

---

## NATS Topology & Messaging

**Diagram: `docs/diagrams/03-nats-topology.excalidraw`**

### Topology: Hub-and-Spoke with Leaf Nodes

```
                    ┌─────────────────────┐
                    │  NATS Core Server    │
                    │  (embedded in hub)   │
                    │                     │
                    │  Client port: 4222  │
                    │  Leaf port:   7422  │
                    └──────────┬──────────┘
                               │
            ┌──────────────────┼──────────────────┐
            │                  │                  │
     ┌──────▼──────┐   ┌──────▼──────┐   ┌──────▼──────┐
     │ Agent leaf   │   │ Agent leaf   │   │ Agent leaf   │
     │ us-east      │   │ fr-par       │   │ ap-south     │
     │ connects to  │   │ connects to  │   │ connects to  │
     │ hub:7422     │   │ hub:7422     │   │ hub:7422     │
     └──────────────┘   └──────────────┘   └──────────────┘
                    ... (46 total agents)
```

Each agent is a **NATS client** (not a NATS server). It connects outbound to the hub's leaf port (7422). The hub's NATS core server routes messages between all connected clients transparently.

This means:
- Agent in `us-east` can send a NATS Request to subject `latency.probe.request.fr-par`
- The hub routes it to the agent in `fr-par` (which is subscribed to that subject)
- `fr-par` replies, and the reply routes back through the hub to `us-east`
- The round-trip time includes: `us-east → hub → fr-par → hub → us-east`

### Subject Namespace

```
latency.
├── probe.
│   └── request.<region>          # Request/Reply — "ping me"
│                                 # Agent subscribes to own region
│                                 # Other agents send Request() to measure RTT
│
├── results.<source>.<target>     # Pub/Sub — measurement results
│                                 # Agent publishes after measuring a target
│                                 # Hub subscribes to latency.results.> (wildcard)
│
├── control.
│   ├── schedule                  # Pub/Sub — hub tells all agents to sweep
│   └── probe.<region>            # Pub/Sub — hub tells one agent to probe one target
│
└── health.<region>               # Pub/Sub — agent heartbeat every 60s
```

### What the latency actually measures

The NATS Request/Reply RTT captures the **full application-layer round trip**:

```
Time ─────────────────────────────────────────────────────►

Agent A (us-east)                Hub (us-ord)              Agent B (fr-par)
    │                               │                          │
    │──── NATS Request() ──────────►│                          │
    │     (TCP to hub:7422)         │──── route to leaf ──────►│
    │                               │     (TCP to fr-par)      │
    │                               │                          │── handle
    │                               │◄──── NATS Reply ─────────│   (instant)
    │◄──── Reply routed back ───────│                          │
    │                               │                          │
    ├──── measured RTT ─────────────┼──────────────────────────┤
```

This RTT includes:
- TCP transit: `us-east → us-ord` (network latency)
- Hub NATS routing overhead (~0.1ms)
- TCP transit: `us-ord → fr-par` (network latency)
- Agent B processing time (~0.01ms)
- Return path (same legs)

**Important**: This is NOT the direct `us-east ↔ fr-par` latency. It's `us-east ↔ us-ord ↔ fr-par` — the path goes through the hub. For regions close to the hub (other US regions), this is approximately correct. For regions far from the hub (e.g., `ap-southeast` Sydney), the hub adds ~50-100ms of overhead each way.

**This is a known trade-off.** Direct measurement would require full mesh connectivity. For a demo tool showing relative latency differences, hub-routed measurement is sufficient. The UI should note that measurements include hub routing.

---

## Latency Measurement

**Diagram: `docs/diagrams/04-measurement-cycle.excalidraw`**

### Sweep Cycle

Every **15 minutes**, the hub triggers a full measurement sweep:

```
T=0:00    Hub publishes latency.control.schedule
          Payload: {"regions": ["us-ord","us-east",...all 46...]}

T=0:02    Agent us-ord wakes (stagger = hash("us-ord") % 30 = 2 seconds)
T=0:14    Agent fr-par wakes (stagger = hash("fr-par") % 30 = 14 seconds)
T=0:27    Agent ap-south wakes (stagger = hash("ap-south") % 30 = 27 seconds)
          ...agents wake at different times to avoid burst

T=0:02    Agent us-ord starts measuring:
          → Request latency.probe.request.us-east    (5 samples, 3s timeout each)
          → Request latency.probe.request.us-central (5 samples)
          → Request latency.probe.request.fr-par     (5 samples)
          → ... (44 more targets, sequential)

T=~4:00   Agent us-ord finishes sweep (~4 min for 45 targets × 5 samples)
          Publishes 45 results to latency.results.us-ord.<target>

T=~5:00   All agents finish (staggered start + ~4min sweep)
          Hub has received ~2,070 results (46 agents × 45 targets)
          1,035 unique pairs, measured in both directions
```

### Per-Target Measurement

For each target region, the agent:

1. Sends **5 NATS Request/Reply** calls (sequential, not parallel)
2. Each has a **3-second timeout**
3. Records RTT for each successful reply
4. Computes: **min, max, median (P50), and mean**
5. Publishes the result as JSON

```json
{
  "source": "us-east",
  "target": "fr-par",
  "rtt_ms": 142.35,
  "min_ms": 140.12,
  "max_ms": 148.91,
  "p50_ms": 141.87,
  "samples": 5,
  "timestamp": "2026-03-18T12:04:22Z"
}
```

### Why 5 samples? Why sequential?

- 5 samples is enough to get a stable median while keeping sweep time reasonable
- Sequential probes avoid creating burst traffic that could skew measurements
- 45 targets × 5 samples × ~100ms avg = ~22 seconds per sweep (in practice)
- Parallel probes would complete faster but risk congestion at the NATS hub

### Client-to-Region Measurement

Separate from NATS measurement. The browser measures HTTP round-trip time to each agent's `/ping` endpoint:

```
Browser                          Agent (fr-par:8080)
   │                                │
   │── fetch("/ping") ─────────────►│
   │   performance.now() = T1       │── returns {"region":"fr-par","ts":...}
   │◄── HTTP 200 ──────────────────│
   │   performance.now() = T2       │
   │                                │
   │   RTT = T2 - T1               │
```

- Warmup request first (discard) to establish TCP/TLS
- 3 measured samples, take median
- Results shown as arcs from "You" to each region on the globe

**Current limitation**: The frontend currently measures to the hub's `/ping` for all regions (same endpoint, same latency). Per-region measurement requires agent endpoints to be publicly routable — documented as a deployment step.

---

## Data Flow

**Diagram: `docs/diagrams/05-data-flow.excalidraw`**

### End-to-end flow from measurement to pixel

```
1. SCHEDULE           2. PROBE              3. AGGREGATE         4. RENDER
Hub timer fires  →    Agents measure   →    Hub collects    →    Frontend displays
                      via NATS Req/Rep      into matrix          on globe

┌─────────────┐      ┌──────────────┐      ┌──────────────┐     ┌──────────────┐
│ Hub          │      │ Agent A      │      │ Hub          │     │ Browser      │
│ Scheduler    │      │ (us-east)    │      │ Aggregator   │     │ (Globe.gl)   │
│              │      │              │      │              │     │              │
│ Publish:     │─────►│ Request:     │      │ Subscribe:   │     │ GET /api/v1/ │
│ control.     │      │ probe.req.   │      │ results.>    │     │ matrix       │
│ schedule     │      │ fr-par       │      │              │     │              │
│              │      │              │      │ Update:      │     │ or           │
│              │      │ Measure RTT  │      │ in-memory    │     │              │
│              │      │              │      │ matrix       │     │ WS /ws/live  │
│              │      │ Publish:     │─────►│              │────►│              │
│              │      │ results.     │      │ Broadcast:   │     │ Update arcs  │
│              │      │ us-east.     │      │ WS clients   │     │ Update stats │
│              │      │ fr-par       │      │              │     │              │
└──────────────┘      └──────────────┘      └──────────────┘     └──────────────┘
```

### Two paths for data to reach the frontend:

**Path A — Polling (REST)**
- Frontend calls `GET /api/v1/matrix` every 30 seconds
- Gets the full latest matrix (all measured pairs)
- Simple, stateless, works through proxies/CDN

**Path B — Streaming (WebSocket)**
- Frontend connects to `WS /ws/live`
- On connect, receives a full snapshot of current matrix
- Then receives individual results as they arrive in real-time
- Lower latency, but requires persistent connection

Both paths deliver the same data. The frontend uses both: WebSocket for live updates, polling as a fallback and to catch anything missed.

---

## API Surface

All endpoints are on the hub server. No authentication on read endpoints (data is not sensitive). Write endpoints (`trigger-sweep`) use query-string token auth.

### Endpoints

| Method | Path | Purpose | Response |
|--------|------|---------|----------|
| `GET` | `/api/v1/matrix` | Full latency matrix (latest values) | `{results: [...], count: N}` |
| `GET` | `/api/v1/matrix?from=us-ord` | Single source row | `{results: [...], count: N}` |
| `GET` | `/api/v1/pair/{from}/{to}` | Single pair + 24h history | `{latest: {...}, history: [...]}` |
| `GET` | `/api/v1/regions` | All 46 regions with lat/lon | `[{id, label, country, lat, lon, short}, ...]` |
| `GET` | `/api/v1/nearest?lat=X&lon=Y` | Top 10 nearest regions by distance | `{client_lat, client_lon, nearest: [...]}` |
| `GET` | `/api/v1/az-pairs?primary=us-ord` | AZ pair recommendations | `{primary, pairs: [{secondary, rtt_ms, category, ...}]}` |
| `GET` | `/api/v1/health` | System status | `{status, nats_connected, pairs_measured, sources}` |
| `GET` | `/api/v1/trigger-sweep?auth=TOKEN` | Force immediate sweep | `{status: "sweep triggered"}` |
| `WS` | `/ws/live` | Real-time latency stream | Snapshot on connect, then individual results |
| `GET` | `/ping` | Hub ping for client latency | `{region: "us-ord", role: "hub", ts: ...}` |

### Matrix response shape

```json
{
  "results": [
    {
      "source": "us-east",
      "target": "fr-par",
      "rtt_ms": 142.35,
      "min_ms": 140.12,
      "max_ms": 148.91,
      "p50_ms": 141.87,
      "samples": 5,
      "timestamp": "2026-03-18T12:04:22Z"
    }
  ],
  "count": 2070,
  "ts": 1710756000
}
```

### AZ Pairs response shape

```json
{
  "primary": "us-ord",
  "pairs": [
    {
      "secondary": "us-iad",
      "rtt_ms": 18.4,
      "distance_km": 940,
      "category": "async-low-rpo",
      "same_country": true,
      "co_located": false,
      "rationale": "Same country, moderate latency — async replication with low RPO"
    }
  ]
}
```

### AZ Category classification

| Category | RTT Threshold | Use Case |
|----------|--------------|----------|
| `sync-capable` | < 10ms | Synchronous replication (same metro or very close cities) |
| `async-low-rpo` | 10–50ms | Async replication with low RPO (same country, adjacent regions) |
| `dr-only` | > 50ms | Disaster recovery only (cross-continent) |

Additional heuristic: if distance < 50km, pair is flagged as `co_located` — same metro, limited fault isolation (e.g., `fr-par` and `fr-par-2` are in the same city).

---

## Frontend & UI

**Diagram: `docs/diagrams/06-ui-layout.excalidraw`**

### Technology

- **Globe.gl v2.31** — Three.js wrapper for rendering data on a 3D globe
- **No framework** — Vanilla JavaScript, single HTML file, inline CSS
- **No build step** — Globe.gl loaded from CDN, everything else is inline
- Embedded in the Go binary via `//go:embed` and served as a static file

### Layout

```
┌─────────────────────────────────────────────────────────────┐
│ ┌───────────────┐                      ┌──────────────────┐ │
│ │ Info Panel     │                      │ Controls Panel   │ │
│ │               │                      │                  │ │
│ │ Title         │     SPINNING 3D      │ [Globe]  active  │ │
│ │ Regions: 46   │       GLOBE          │ [Heatmap]        │ │
│ │ Pairs: 2070   │                      │ [Client Latency] │ │
│ │ Avg: 98ms     │   Regions = dots     │                  │ │
│ │ Sweep: 2m ago │   Latency = arcs     │ [AZ Selector]    │ │
│ │               │   Color = RTT        │ [Region Finder]  │ │
│ └───────────────┘                      └──────────────────┘ │
│                                                             │
│                                                             │
│                                                             │
│ ┌───────────────┐                      ┌──────────────────┐ │
│ │ Detail Panel  │                      │ Tool Panel       │ │
│ │               │                      │                  │ │
│ │ Shows when    │                      │ Shows when AZ    │ │
│ │ region is     │                      │ or Finder mode   │ │
│ │ clicked       │                      │ is active        │ │
│ └───────────────┘                      └──────────────────┘ │
│                                                             │
│            ┌──────────────────────────────┐                 │
│            │ Legend: ● <50ms ● 50-150ms ● >150ms │          │
│            └──────────────────────────────┘                 │
└─────────────────────────────────────────────────────────────┘
```

### Views

**Globe View** (default)
- Auto-rotating 3D globe with NASA Blue Marble texture
- 46 region markers (green dots)
- Latency arcs between measured pairs
- Arc color: green (<50ms), yellow (50–150ms), red (>150ms)
- Arc height proportional to latency (higher arc = more latency)
- Click a region → highlights that region's arcs, shows detail panel

**Heatmap View**
- Modal overlay with a canvas-rendered 46×46 matrix
- Rows and columns labeled with region short names
- Cell color intensity maps to RTT
- Hover shows exact value in tooltip

**Client Latency View**
- Browser fires `fetch()` to each region's `/ping` endpoint
- Warmup request, then 3 samples, take median
- Results shown as arcs from "You" (via browser geolocation) to each region
- Sorted list in detail panel (fastest first)

**AZ Pair Selector**
- Dropdown to pick a primary region
- Calls `/api/v1/az-pairs?primary=X`
- Results grouped by category (sync / async / DR)
- Shows arcs on globe from primary to top candidates

**Region Finder**
- Enter lat/lon or click "Use My Location"
- Calls `/api/v1/nearest?lat=X&lon=Y`
- Shows top 10 nearest regions by geographic distance
- Arcs from query point to nearest regions on globe

---

## Deployment Model

**Diagram: `docs/diagrams/07-deployment.excalidraw`**

### Infrastructure

| Resource | Spec | Region | Monthly Cost |
|----------|------|--------|-------------|
| Hub VM | `g6-standard-2` (4GB, 2 vCPU) | `us-ord` | $24 |
| 46 Agent VMs | `g6-nanode-1` (1GB, 1 vCPU) | One per Linode region | $230 |
| DNS | A record: `latency.connected-cloud.io` | Akamai Edge DNS | included |
| **Total** | | | **$254/mo** |

### How deployment works

All infrastructure is managed by Terraform (`infra/terraform/`).

```
Developer machine                     Linode Cloud
┌──────────────────┐
│ make build-agent-linux              46× Nanode
│ make build-hub                      ┌──────────────┐
│                    ───terraform────►│ latency-agent │ (systemd)
│ terraform apply    apply            │ :8080 /ping   │
│                                     └──────────────┘
│
│                                     1× Standard-2
│                                     ┌──────────────┐
│                    ───terraform────►│ latency-hub   │ (systemd)
│                    apply            │ :443 API+Web  │
│                                     │ :4222 NATS    │
│                                     │ :7422 leaf    │
│                                     └──────────────┘
└──────────────────┘
```

1. Cross-compile Go binaries locally (`make build-agent-linux`, `make build-hub`)
2. Terraform creates 47 Linode instances (1 hub + 46 agents)
3. Terraform uploads binaries via SSH (`file` provisioner)
4. Terraform creates systemd units and starts services (`remote-exec` provisioner)
5. Terraform creates firewalls and DNS records
6. Agents connect outbound to hub's NATS leaf port
7. First measurement sweep triggers 10 seconds after hub starts

### Teardown

`terraform destroy` removes all instances, firewalls, and DNS records. Reminder to check for orphaned NodeBalancers and Volumes (standard LKE cleanup pattern).

### On-demand usage

Deploy for a demo, destroy after. `make up` and `make down` are aliases. Full deploy takes ~5 minutes; full teardown takes ~2 minutes.

---

## Security Model

### Network

- **Hub firewall**: HTTPS (443) open to public, NATS leaf (7422) open to agent IPs, SSH restricted to admin IP
- **Agent firewall**: HTTP ping (8080) open to public (needed for client latency measurement), SSH restricted to admin IP
- All other ports DROP by default

### Authentication

- **API read endpoints**: Unauthenticated (latency data is not sensitive)
- **API write endpoints**: Query-string token auth (`?auth=TOKEN`)
- **NATS**: Username/password in connection URL (v1 — mTLS planned for v2)
- **SSH**: Key-based only (no passwords)

### Known risks (v1, accepted)

- NATS credentials visible in systemd unit files (readable by root; these are single-purpose VMs)
- No TLS on NATS leaf connections (traffic is latency measurements, not sensitive data)
- CORS allows all origins (required for client latency measurement from any browser)

---

## Cost Model

### Running (all 46 regions)

```
46 × Nanode ($5)    = $230/mo
 1 × Standard-2     =  $24/mo
 DNS                 =   $0 (included with Akamai)
─────────────────────────────
 Total               = $254/mo
```

### Partial deployment (for testing)

The `agent_regions` Terraform variable can be overridden to deploy a subset:

```hcl
# Deploy only 5 US regions for testing
agent_regions = {
  "us-ord"  = { label = "probe-ord" }
  "us-east" = { label = "probe-ewr" }
  "us-west" = { label = "probe-fmt" }
  "fr-par"  = { label = "probe-par" }
  "ap-south" = { label = "probe-sin" }
}
```

Cost: 5 × $5 + $24 = **$49/mo** for a 5-region test.

### NATS bandwidth

- ~10,350 NATS messages per sweep (46 agents × 45 targets × 5 samples)
- Each message is <1KB
- At 15-min intervals: ~11.5 messages/second sustained
- Well within single NATS server capacity (~10M messages/second)
