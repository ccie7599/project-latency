# DECISIONS.md — project-latency

## ADR-001: NATS Hub-and-Spoke Topology
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need to measure latency between 46 regions. Options: full mesh (gateways), hub-and-spoke (leaf nodes), or direct TCP/ICMP probes.

**Decision**: Hub-and-spoke with NATS leaf nodes connecting to a single core server in us-ord.

**Rationale**:
- Full mesh of 46 NATS servers would require 1,035 gateway connections — operationally complex
- Leaf nodes are simple: each agent connects to one URL, hub routes transparently
- Measures actual NATS application-layer latency (not just ICMP), which is what customers care about for distributed apps
- Proven pattern from sse-cdn project

**Consequences**: Hub is a SPOF. Latency measurements include hub routing overhead (adds ~1-2ms). Acceptable for demo use.

---

## ADR-002: Globe.gl for 3D Visualization
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need a spinning 3D globe with interactive region markers and latency arcs.

**Decision**: Globe.gl (built on Three.js) with vanilla JS and Vite.

**Rationale**:
- Purpose-built for data-on-globe visualization
- Supports arcs, points, labels, heatmaps out of the box
- Lightweight compared to Cesium or full Three.js scenes
- No heavy framework needed — vanilla JS keeps bundle small

**Consequences**: Limited to globe projection (no flat map fallback). Acceptable for this use case.

---

## ADR-003: Bare Nanodes with systemd (No K8s/Docker)
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need to deploy lightweight agents to 46 regions.

**Decision**: Cross-compiled Go binary deployed to bare Nanodes ($5/mo each), managed by systemd, provisioned via Terraform.

**Rationale**:
- Go binary is ~15MB, runs with <100MB RAM — Docker/K8s is unnecessary overhead
- Nanodes have 1GB RAM — plenty for a static Go binary + embedded NATS leaf
- systemd provides restart-on-failure, logging, and resource limits
- Terraform provisioners handle deployment (file upload + remote-exec)

**Consequences**: No container orchestration. Updates require re-provisioning or a simple SCP + systemctl restart. Acceptable for a demo tool with infrequent updates.

---

## ADR-004: In-Memory + SQLite for Latency Storage
**Date**: 2026-03-18
**Status**: Accepted

**Context**: Need to store latency matrix (latest + 24h history).

**Decision**: Primary store is in-memory Go map. SQLite for persistence across hub restarts.

**Rationale**:
- 1,035 pairs × 96 samples/day × ~200 bytes = ~20MB — easily fits in memory
- SQLite adds durability without external dependencies
- No need for Prometheus/InfluxDB complexity for a demo tool

**Consequences**: History limited to what fits in SQLite on a single node. No distributed querying. Acceptable.
