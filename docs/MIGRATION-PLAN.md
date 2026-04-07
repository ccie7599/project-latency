# Migration Plan: Latency Demo → Presales Landing Zone

**Date**: 2026-04-07
**Status**: Approved

## Architecture Summary

**Hybrid deployment across two Linode accounts:**

| Tier | Account | Regions | Infra | Agent Deploy | Metrics Export |
|------|---------|---------|-------|-------------|----------------|
| LKE clusters | Presales | 32 core | LKE (g6-standard-1, 1 node) | K8s Deployment | OTel Agent → hub gateway |
| Bare VMs | Old (demo) | 12 distributed + 2 core | g6-dedicated-edge-2 / g6-nanode-1 | systemd binary | Grafana Alloy → hub gateway OTLP |
| Hub | Presales | us-ord (central LKE) | Namespace in landing zone cluster | K8s Deployment | Local (same cluster) |

**Total: 46 regions** (32 LKE + 14 VM), full NATS mesh, centralized telemetry

### Cost

| Component | Count | Unit Cost | Total |
|-----------|-------|-----------|-------|
| LKE node pools (g6-standard-1) | 32 | $12/mo | $384/mo |
| LKE control plane (standard) | 32 | $0 | $0 |
| Distributed VMs (g6-dedicated-edge-2) | 12 | $43/mo | $516/mo |
| Core VMs — old account (g6-nanode-1) | 2 | $5/mo | $10/mo |
| Hub (runs in existing landing zone LKE) | 1 | $0 | $0 |
| **Total** | | | **$910/mo** |

Central landing zone cluster ($336/mo) is already paid — not incremental.

---

## Phase 1: Hub in Landing Zone Cluster

**Goal**: Deploy the hub (API + frontend + NATS seed + Prometheus scraper) as a K8s workload in the central landing zone LKE cluster

**Tasks**:
1. Create namespace `latency-demo` in the landing zone cluster
2. Build hub container image, push to Harbor registry
3. Create K8s Deployment (1 replica, hostNetwork for NATS 6222 + monitoring 8222)
4. Hub's Prometheus metric endpoint (`:2112`) — OTel agent in the cluster auto-scrapes it
5. Hub PromQL queries point to `http://prometheus.central-services.svc.cluster.local:9090`
6. Expose hub HTTPS (`:443`) via hostPort for Akamai origin pull
7. Mount TLS cert (LE wildcard for `*.connected-cloud.io`) as K8s secret
8. Configure `AUTH_TOKEN` from Vault
9. New origin DNS: `latency-origin.presales.connected-cloud.io` → node IP

**Patterns to copy**:
- LKE deployment: `~/project-landing-zone/presales-landing-zone/infra/k8s/central-services/`
- Vault secret injection: `~/federated-observability/edge/vault-secrets-operator/`
- hostPort ingress: per CLAUDE.md global prefs

**Verification**:
- [ ] `curl https://latency-origin.presales.connected-cloud.io/api/v1/health?auth=TOKEN` → 200
- [ ] Hub NATS server running, accepting leaf/cluster connections on 6222
- [ ] Prometheus metrics visible in central Grafana

---

## Phase 2: LKE Clusters (32 Core Regions)

**Goal**: One LKE cluster per core region, single-node pool, running NATS agent

**Tasks**:
1. Terraform module `infra/terraform/lke-agents/` — creates 32 `linode_lke_cluster` resources using presales account token
2. Each cluster: `g6-standard-1` (2GB, 1 vCPU), 1 node, k8s 1.35, standard control plane (free)
3. Cloud Firewall per cluster:
   - 6222: cluster member IPs only (hub + all agents)
   - 8222: hub IP only
   - 443: hub IP + admin IP (for /ping)
   - 22: admin IP only
4. Agent container image (same binary as hub, different entrypoint)
5. K8s Deployment per cluster: 1 replica, hostNetwork, NATS seed route → hub external IP
6. Mount TLS cert for HTTPS `/ping` endpoint

**Key config**:
```hcl
resource "linode_lke_cluster" "agent" {
  for_each    = var.lke_regions
  label       = "latency-${each.value.short}"
  region      = each.key
  k8s_version = "1.35"
  
  pool {
    type  = "g6-standard-1"
    count = 1
  }
}
```

**Verification**:
- [ ] 32 clusters in `Ready` state
- [ ] NATS agent pod `Running` in each
- [ ] Hub `/routez` shows all 32 members connected

---

## Phase 3: OTel Telemetry Federation (LKE Clusters)

**Goal**: Ship NATS metrics from each LKE cluster back to central Prometheus via OTel

**Tasks**:
1. Deploy OTel Agent DaemonSet in each LKE cluster (same image as landing zone: `otel/opentelemetry-collector-contrib:0.96.0`)
2. Agent config:
   - Receivers: `kubeletstats`, `prometheus` (scrape local NATS `:8222/varz`)
   - Processors: `k8sattributes`, `resource` (add `cluster.id` = region ID), `batch`
   - Exporters: `otlp/hub` → hub gateway LoadBalancer IP on 4317
3. mTLS certs issued by cert-manager in hub cluster, distributed via Vault Secrets Operator
4. Resource labels: `cluster.id`, `cluster.role=latency-agent`, `region`

**Patterns to copy**:
- Edge agent config: `~/federated-observability/edge/agent-config.yaml`
- VSO setup: `~/federated-observability/edge/vault-secrets-operator/`
- Hub gateway already running and accepting OTLP on its LoadBalancer

**Verification**:
- [ ] `up{cluster_id="us-east"}` returns data in Grafana
- [ ] NATS route RTT visible as Prometheus metric per cluster
- [ ] No OTel pods in CrashLoopBackOff

---

## Phase 4: Distributed Region VMs (14 Regions, Old Account)

**Goal**: Keep existing VMs in old account, add Grafana Alloy for metrics export, re-point NATS seed to new hub

**Tasks**:
1. Update NATS seed route on all 14 VMs → new hub external IP in presales LKE
2. Install Grafana Alloy on each VM (single binary, ~30MB, systemd unit)
3. Alloy config per VM:
   ```yaml
   prometheus.scrape "nats" {
     targets = [{"__address__" = "localhost:8222"}]
     metrics_path = "/varz"
     scrape_interval = "15s"
   }
   otelcol.exporter.otlp "hub" {
     client {
       endpoint = "<hub-gateway-lb>:4317"
       tls { insecure = true }  # or mTLS if we set up certs
     }
   }
   ```
4. Update cloud firewalls in old account:
   - 6222: add new hub IP + all new LKE agent IPs
   - Remove old hub IP from rules
5. Verify mesh connectivity across accounts

**Regions** (14 total):
- Distributed (12): us-den-1, us-hou-1, mx-qro-1, co-bog-1, co-bog-2, cl-scl-1, de-ber-1, de-ham-1, fr-mrs-2, my-kul-1, nz-akl-1, za-jnb-1
- Core in old account only (2): us-iad-2, no-osl-1

**Verification**:
- [ ] Hub `/routez` shows all 46 members (32 LKE + 14 VM)
- [ ] Alloy metrics from distributed VMs visible in central Prometheus
- [ ] NATS RTT measurements include cross-account routes

---

## Phase 5: Akamai Property Migration

**Goal**: Point `latency-demo.connected-cloud.io` at the new hub origin

**Tasks**:
1. Create DNS A record: `latency-origin.presales.connected-cloud.io` → hub node IP
2. Obtain LE cert for the origin hostname (cert-manager in LKE or acme.sh)
3. Update Akamai property origin in Terraform:
   - Old: `latency.connected-cloud.io`
   - New: `latency-origin.presales.connected-cloud.io`
4. `terraform apply` — re-activates property with new origin (staging then production)
5. Per-region DNS records: update `{short}.latency.connected-cloud.io` A records → new LKE node IPs + keep old VM IPs for distributed regions
6. DS2 stream: no change (same property ID, same webhook)

**No change needed**:
- SAN cert on edge hostname (enrollment 293468) — already covers `latency-demo.connected-cloud.io`
- Token auth — same mechanism
- Property rules — same caching/WS/DS2 config

**Verification**:
- [ ] `curl -sI https://latency-demo.connected-cloud.io/ping` → 200, Akamai headers present, new origin IP
- [ ] DS2 data flowing to ClickHouse
- [ ] UI loads, WebSocket connects, matrix populated

---

## Phase 6: Decommission Old Hub + Core Agents

**Goal**: Remove old hub VM and the 32 core-region VMs from the old account (keep 14 distributed/missing VMs)

**Tasks**:
1. Terraform targeted destroy: remove hub + 32 core agent modules from old account state
2. Delete old DNS A records for hub (`latency.connected-cloud.io`) and core agents
3. Delete old firewalls for removed instances
4. Clean up orphaned resources: `linode-cli nodebalancers list && linode-cli volumes list`
5. Update old account Terraform to only manage the 14 remaining VMs

**Keep running in old account**:
- 14 VMs (12 distributed + 2 core) with NATS agent + Alloy
- Their firewalls
- Their DNS records (`{short}.latency.connected-cloud.io`)

**Verification**:
- [ ] Old account: only 14 instances remain
- [ ] Full mesh still 46 nodes
- [ ] No orphaned resources

---

## Phase 7: Documentation + Cleanup

**Tasks**:
- [ ] Update SCOPE.md — hybrid architecture, 46 regions, two accounts, cost model
- [ ] Update README.md — new deployment instructions
- [ ] ADR in DECISIONS.md — LKE migration, hybrid VM+K8s, Alloy for VM telemetry
- [ ] Update Webex intro — region count, architecture note
- [ ] Update architecture diagrams

---

## Execution Order

1. **Phase 1** first — get the hub running in the landing zone cluster, verify it works standalone
2. **Phase 2** — start with 3 US LKE clusters as proof of concept, then expand to all 32
3. **Phase 3** — OTel federation, can overlap with Phase 2 expansion
4. **Phase 4** — re-point distributed VMs to new hub, install Alloy
5. **Phase 5** — Akamai cutover (brief DNS propagation window)
6. **Phase 6** — decommission old core infra after 24h soak
7. **Phase 7** — docs

**Estimated timeline**: 2-3 sessions
