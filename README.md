# Linode Global Latency Map

Interactive 3D globe visualization showing real-time inter-region latency between all 46 Linode regions, measured via NATS request/reply.

## Features

- **Spinning Globe** — 46 Linode regions with latency arcs color-coded by RTT (green <50ms, yellow 50-150ms, red >150ms)
- **Heatmap Matrix** — 46x46 latency matrix with hover tooltips
- **Client Latency** — Measure your browser's latency to every region
- **AZ Pair Selector** — Find optimal region pairs for sync replication, async replication, or DR
- **Region Finder** — Enter coordinates or use geolocation to find nearest Linode regions
- **Live Updates** — WebSocket-driven real-time updates as measurements flow in

## Architecture

```
┌──────────────────────────────────────────────────┐
│  Browser (Globe.gl)                              │
│  ├── REST API polling                            │
│  ├── WebSocket live updates                      │
│  └── HTTP /ping to all 46 agents                 │
└──────────┬───────────────────────────────────────┘
           │
┌──────────▼───────────────────────────────────────┐
│  Hub (us-ord)                                     │
│  ├── NATS Core Server (:4222 client, :7422 leaf)  │
│  ├── REST API + WebSocket (:443)                  │
│  ├── Aggregator (subscribes latency.results.>)    │
│  └── Static frontend (embed.FS)                   │
└──────────┬───────────────────────────────────────┘
           │ NATS leaf connections
    ┌──────┴──────┬──────────┬──────── ... ──┐
    ▼             ▼          ▼               ▼
┌────────┐  ┌────────┐  ┌────────┐     ┌────────┐
│Agent   │  │Agent   │  │Agent   │     │Agent   │
│us-east │  │fr-par  │  │ap-south│     │za-jnb  │
│Nanode  │  │Nanode  │  │Nanode  │     │Nanode  │
└────────┘  └────────┘  └────────┘     └────────┘
  46 regions, NATS request/reply probes every 15 min
```

## Quick Start

### Local Development

```bash
# Build and run hub
make deps
make build-hub
LISTEN_ADDR=:8080 ./bin/hub

# In another terminal, run a test agent
REGION=us-ord NATS_URL=nats://127.0.0.1:4222 LISTEN_ADDR=:9091 go run ./cmd/agent

# Open http://localhost:8080
```

### Full Deployment (46 regions)

```bash
# Cross-compile agent for Linux
make build-agent-linux
make build-hub

# Configure Terraform
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values

# Deploy
terraform init
terraform apply

# Tear down when done
terraform destroy
```

## Cost

| Component | Spec | Count | $/mo |
|-----------|------|-------|------|
| Hub | g6-standard-2 | 1 | $24 |
| Agents | g6-nanode-1 | 46 | $230 |
| **Total** | | | **$254** |

Use `make down` / `make up` to destroy/recreate for on-demand demos.

## API

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/matrix` | Full latency matrix |
| `GET /api/v1/matrix?from=us-ord` | Single source row |
| `GET /api/v1/pair/{from}/{to}` | Pair detail + history |
| `GET /api/v1/regions` | All region metadata |
| `GET /api/v1/nearest?lat=X&lon=Y` | Nearest regions |
| `GET /api/v1/az-pairs?primary=us-ord` | AZ pair recommendations |
| `GET /api/v1/health` | System health |
| `WS /ws/live` | Live latency stream |
