locals {
  # Distributed (edge) regions require g6-dedicated-edge-2 ($43/mo, smallest available)
  # Core regions use g6-nanode-1 ($5/mo)
  distributed_regions = toset([
    "nz-akl-1", "us-den-1", "de-ham-1", "za-jnb-1", "my-kul-1",
    "co-bog-1", "mx-qro-1", "us-hou-1", "cl-scl-1", "de-ber-1",
    "co-bog-2", "fr-mrs-2",
  ])

  # All 46 Linode regions
  all_regions = {
    "us-ord"        = { label = "probe-ord",  short = "ord"  }
    "us-east"       = { label = "probe-ewr",  short = "ewr"  }
    "us-central"    = { label = "probe-dfw",  short = "dfw"  }
    "us-west"       = { label = "probe-fmt",  short = "fmt"  }
    "us-southeast"  = { label = "probe-atl",  short = "atl"  }
    "us-lax"        = { label = "probe-lax",  short = "lax"  }
    "us-mia"        = { label = "probe-mia",  short = "mia"  }
    "us-sea"        = { label = "probe-sea",  short = "sea"  }
    "us-iad"        = { label = "probe-iad",  short = "iad"  }
    "us-iad-2"      = { label = "probe-iad2", short = "iad2" }
    "us-den-1"      = { label = "probe-den",  short = "den"  }
    "us-hou-1"      = { label = "probe-hou",  short = "hou"  }
    "ca-central"    = { label = "probe-yyz",  short = "yyz"  }
    "br-gru"        = { label = "probe-gru",  short = "gru"  }
    "co-bog-1"      = { label = "probe-bog1", short = "bog1" }
    "co-bog-2"      = { label = "probe-bog2", short = "bog2" }
    "mx-qro-1"      = { label = "probe-qro",  short = "qro"  }
    "cl-scl-1"      = { label = "probe-scl",  short = "scl"  }
    "eu-west"       = { label = "probe-lon",  short = "lon"  }
    "gb-lon"        = { label = "probe-lon2", short = "lon2" }
    "eu-central"    = { label = "probe-fra",  short = "fra"  }
    "de-fra-2"      = { label = "probe-fra2", short = "fra2" }
    "fr-par"        = { label = "probe-par",  short = "par"  }
    "fr-par-2"      = { label = "probe-par2", short = "par2" }
    "nl-ams"        = { label = "probe-ams",  short = "ams"  }
    "se-sto"        = { label = "probe-sto",  short = "sto"  }
    # "es-mad" excluded — region currently restricted for new instances
    "it-mil"        = { label = "probe-mil",  short = "mil"  }
    "de-ber-1"      = { label = "probe-ber",  short = "ber"  }
    "de-ham-1"      = { label = "probe-ham",  short = "ham"  }
    "no-osl-1"      = { label = "probe-osl",  short = "osl"  }
    "fr-mrs-2"      = { label = "probe-mrs",  short = "mrs"  }
    "ap-south"      = { label = "probe-sin",  short = "sin"  }
    "sg-sin-2"      = { label = "probe-sin2", short = "sin2" }
    "ap-northeast"  = { label = "probe-tyo2", short = "tyo2" }
    "jp-tyo-3"      = { label = "probe-tyo3", short = "tyo3" }
    "jp-osa"        = { label = "probe-osa",  short = "osa"  }
    "ap-west"       = { label = "probe-bom",  short = "bom"  }
    "in-bom-2"      = { label = "probe-bom2", short = "bom2" }
    "in-maa"        = { label = "probe-maa",  short = "maa"  }
    "id-cgk"        = { label = "probe-cgk",  short = "cgk"  }
    "my-kul-1"      = { label = "probe-kul",  short = "kul"  }
    "ap-southeast"  = { label = "probe-syd",  short = "syd"  }
    "au-mel"        = { label = "probe-mel",  short = "mel"  }
    "nz-akl-1"      = { label = "probe-akl",  short = "akl"  }
    "za-jnb-1"      = { label = "probe-jnb",  short = "jnb"  }
  }

  agent_regions = var.agent_regions != null ? var.agent_regions : local.all_regions
  common_tags   = ["project:latency", "owner:brian"]
}

# ============================================================
# Hub Server — NATS cluster seed + API + Prometheus
# ============================================================
resource "linode_instance" "hub" {
  label     = "latency-hub"
  region    = var.hub_region
  type      = "g6-standard-2"
  image     = "linode/ubuntu24.04"
  tags      = local.common_tags
  root_pass = null

  authorized_keys = [var.ssh_public_key]

  provisioner "file" {
    source      = var.hub_binary_path
    destination = "/usr/local/bin/latency-hub"
    connection {
      type        = "ssh"
      host        = self.ip_address
      user        = "root"
      private_key = file("~/.ssh/id_ed25519")
    }
  }

  provisioner "remote-exec" {
    inline = [
      "chmod +x /usr/local/bin/latency-hub",

      # Install Prometheus
      "curl -sL https://github.com/prometheus/prometheus/releases/download/v2.53.0/prometheus-2.53.0.linux-amd64.tar.gz | tar xz -C /tmp",
      "cp /tmp/prometheus-*/prometheus /usr/local/bin/",
      "cp /tmp/prometheus-*/promtool /usr/local/bin/",
      "mkdir -p /etc/prometheus /var/lib/prometheus",

      "cat > /etc/prometheus/prometheus.yml << 'PROMCFG'",
      "global:",
      "  scrape_interval: 10s",
      "scrape_configs:",
      "  - job_name: nats-latency",
      "    static_configs:",
      "      - targets: ['localhost:2112']",
      "PROMCFG",

      "cat > /etc/systemd/system/prometheus.service << 'UNIT'",
      "[Unit]",
      "Description=Prometheus",
      "After=network.target",
      "[Service]",
      "Type=simple",
      "ExecStart=/usr/local/bin/prometheus --config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/var/lib/prometheus --storage.tsdb.retention.time=15d --web.listen-address=:9090",
      "Restart=always",
      "RestartSec=5",
      "[Install]",
      "WantedBy=multi-user.target",
      "UNIT",

      "cat > /etc/systemd/system/latency-hub.service << 'UNIT'",
      "[Unit]",
      "Description=Linode Latency Hub",
      "After=network.target prometheus.service",
      "[Service]",
      "Type=simple",
      "ExecStart=/usr/local/bin/latency-hub",
      "Restart=always",
      "RestartSec=5",
      "Environment=REGION=${var.hub_region}",
      "Environment=LISTEN_ADDR=:443",
      "Environment=TLS_CERT=/etc/latency/fullchain.pem",
      "Environment=TLS_KEY=/etc/latency/privkey.pem",
      "Environment=METRICS_ADDR=:2112",
      "Environment=PROMETHEUS_URL=http://localhost:9090",
      "Environment=AUTH_TOKEN=${var.auth_token}",
      "[Install]",
      "WantedBy=multi-user.target",
      "UNIT",

      "systemctl daemon-reload",
      "systemctl enable --now prometheus",
      "systemctl enable --now latency-hub",
    ]

    connection {
      type        = "ssh"
      host        = self.ip_address
      user        = "root"
      private_key = file("~/.ssh/id_ed25519")
    }
  }
}

# ============================================================
# Firewalls — one for hub, one shared across all agents
# ============================================================
locals {
  # All cluster member IPs in CIDR notation — hub + every agent
  cluster_ips = concat(
    ["${linode_instance.hub.ip_address}/32"],
    [for mod in module.agent : "${mod.ip_address}/32"]
  )
}

resource "linode_firewall" "hub" {
  label = "latency-hub-fw"
  tags  = local.common_tags

  # HTTPS origin — Akamai OIPACL + admin + this server
  inbound {
    label    = "https-origin"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "443"
    ipv4     = [
      "2.16.0.0/13",       # Akamai OIPACL
      "23.0.0.0/12",       # Akamai OIPACL
      "23.32.0.0/11",      # Akamai OIPACL
      "23.192.0.0/11",     # Akamai OIPACL
      "95.100.0.0/15",     # Akamai OIPACL
      "184.24.0.0/13",     # Akamai OIPACL
      var.admin_ip,         # direct admin access
      "172.234.206.146/32", # this server
    ]
  }

  inbound {
    label    = "nats-cluster"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "6222"
    ipv4     = local.cluster_ips
  }

  inbound {
    label    = "nats-monitor"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8222"
    ipv4     = ["172.234.206.146/32"]
  }

  inbound {
    label    = "ssh"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "22"
    ipv4     = [var.admin_ip, "172.234.206.146/32"]
  }

  inbound_policy  = "DROP"
  outbound_policy = "ACCEPT"

  linodes = [linode_instance.hub.id]
}

resource "linode_firewall" "agents" {
  label = "latency-agents-fw"
  tags  = local.common_tags

  inbound {
    label    = "https-ping"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "443"
    ipv4     = ["${linode_instance.hub.ip_address}/32", var.admin_ip]
  }

  inbound {
    label    = "nats-cluster"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "6222"
    ipv4     = local.cluster_ips
  }

  inbound {
    label    = "nats-monitor"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8222"
    ipv4     = ["${linode_instance.hub.ip_address}/32"]
  }

  inbound {
    label    = "ssh"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "22"
    ipv4     = [var.admin_ip, "172.234.206.146/32"]
  }

  inbound_policy  = "DROP"
  outbound_policy = "ACCEPT"

  linodes = [for mod in module.agent : mod.instance_id]
}

# ============================================================
# Agent Instances — NATS cluster members
# ============================================================
module "agent" {
  for_each = local.agent_regions
  source   = "./modules/agent"

  region           = each.key
  label            = each.value.label
  is_distributed   = contains(local.distributed_regions, each.key)
  nats_seed_routes = "nats-route://${linode_instance.hub.ip_address}:6222"
  binary_path      = var.agent_binary_path
  ssh_key          = var.ssh_public_key
  tags             = local.common_tags
}

# DNS is managed via Akamai Edge DNS (connected-cloud.io zone)
# After apply, use outputs to create A records:
#   latency.connected-cloud.io -> hub_ip
#   {short}.latency.connected-cloud.io -> agent_ips[region]
