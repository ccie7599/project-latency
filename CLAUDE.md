# Project: latency-demo (Global Latency Map)

## ⚠️ LZ compliance debt — 2026-05-27 audit

This deploy violates several shared-services patterns from `~/project-landing-zone/presales-landing-zone/docs/INTAKE.md`. See the per-pattern fix snippets in `~/project-landing-zone/presales-landing-zone/docs/compliance-audit-2026-05-27.md`.

- **Harbor**: `brianapley/latency-hub:latest` is on Docker Hub. Migrate to `harbor.harbor.svc.cluster.local/presales/latency-hub:<git-sha>` and drop `:latest`. INTAKE explicitly flags `:latest` as Argo-incompatible (selfHeal doesn't redeploy on tag-only changes).
- **Argo**: namespace not managed by an Argo Application.
- **Vault**: uses raw k8s `Secret` objects instead of Vault Agent injection. Move to `api/latency-demo/config` and add `vault.hashicorp.com/agent-inject` annotations.

OTel is wired (prom-scrape annotation present), so that's fine.

Estimated cleanup: ~3 hours. Multi-region GTM origin means the Harbor migration needs care — cut over one region at a time. After remediating, delete this section.
