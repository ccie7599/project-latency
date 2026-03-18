locals {
  # All 46 Linode regions
  all_regions = {
    "us-ord"        = { label = "probe-ord" }
    "us-east"       = { label = "probe-ewr" }
    "us-central"    = { label = "probe-dfw" }
    "us-west"       = { label = "probe-fmt" }
    "us-southeast"  = { label = "probe-atl" }
    "us-lax"        = { label = "probe-lax" }
    "us-mia"        = { label = "probe-mia" }
    "us-sea"        = { label = "probe-sea" }
    "us-iad"        = { label = "probe-iad" }
    "us-iad-2"      = { label = "probe-iad2" }
    "us-den-1"      = { label = "probe-den" }
    "us-hou-1"      = { label = "probe-hou" }
    "ca-central"    = { label = "probe-yyz" }
    "br-gru"        = { label = "probe-gru" }
    "co-bog-1"      = { label = "probe-bog1" }
    "co-bog-2"      = { label = "probe-bog2" }
    "mx-qro-1"      = { label = "probe-qro" }
    "cl-scl-1"      = { label = "probe-scl" }
    "eu-west"       = { label = "probe-lon" }
    "gb-lon"        = { label = "probe-lon2" }
    "eu-central"    = { label = "probe-fra" }
    "de-fra-2"      = { label = "probe-fra2" }
    "fr-par"        = { label = "probe-par" }
    "fr-par-2"      = { label = "probe-par2" }
    "nl-ams"        = { label = "probe-ams" }
    "se-sto"        = { label = "probe-sto" }
    "es-mad"        = { label = "probe-mad" }
    "it-mil"        = { label = "probe-mil" }
    "de-ber-1"      = { label = "probe-ber" }
    "de-ham-1"      = { label = "probe-ham" }
    "no-osl-1"      = { label = "probe-osl" }
    "fr-mrs-2"      = { label = "probe-mrs" }
    "ap-south"      = { label = "probe-sin" }
    "sg-sin-2"      = { label = "probe-sin2" }
    "ap-northeast"  = { label = "probe-tyo2" }
    "jp-tyo-3"      = { label = "probe-tyo3" }
    "jp-osa"        = { label = "probe-osa" }
    "ap-west"       = { label = "probe-bom" }
    "in-bom-2"      = { label = "probe-bom2" }
    "in-maa"        = { label = "probe-maa" }
    "id-cgk"        = { label = "probe-cgk" }
    "my-kul-1"      = { label = "probe-kul" }
    "ap-southeast"  = { label = "probe-syd" }
    "au-mel"        = { label = "probe-mel" }
    "nz-akl-1"      = { label = "probe-akl" }
    "za-jnb-1"      = { label = "probe-jnb" }
  }

  # Use custom list or all regions
  agent_regions = var.agent_regions != null ? var.agent_regions : local.all_regions

  common_tags = ["project:latency", "owner:brian"]
}

# ============================================================
# Hub Server
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
      "cat > /etc/systemd/system/latency-hub.service << 'UNIT'",
      "[Unit]",
      "Description=Linode Latency Hub",
      "After=network.target",
      "",
      "[Service]",
      "Type=simple",
      "ExecStart=/usr/local/bin/latency-hub",
      "Restart=always",
      "RestartSec=5",
      "Environment=LISTEN_ADDR=:443",
      "Environment=AUTH_TOKEN=${var.auth_token}",
      "",
      "[Install]",
      "WantedBy=multi-user.target",
      "UNIT",
      "systemctl daemon-reload",
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

resource "linode_firewall" "hub" {
  label = "latency-hub-fw"
  tags  = local.common_tags

  inbound {
    label    = "https"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "443"
    ipv4     = ["0.0.0.0/0"]
    ipv6     = ["::/0"]
  }

  inbound {
    label    = "http"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "80"
    ipv4     = ["0.0.0.0/0"]
    ipv6     = ["::/0"]
  }

  inbound {
    label    = "nats-leaf"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "7422"
    ipv4     = ["0.0.0.0/0"]  # Agents connect from dynamic IPs
    ipv6     = ["::/0"]
  }

  inbound {
    label    = "ssh-admin"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "22"
    ipv4     = [var.admin_ip]
  }

  inbound_policy  = "DROP"
  outbound_policy = "ACCEPT"

  linodes = [linode_instance.hub.id]
}

# ============================================================
# Agent Instances (one per region)
# ============================================================
module "agent" {
  for_each = local.agent_regions
  source   = "./modules/agent"

  region       = each.key
  label        = each.value.label
  nats_url     = "nats://${var.nats_user}:${var.nats_pass}@${linode_instance.hub.ip_address}:7422"
  binary_path  = var.agent_binary_path
  ssh_key      = var.ssh_public_key
  admin_ip     = var.admin_ip
  tags         = local.common_tags
}

# ============================================================
# DNS
# ============================================================
data "linode_domain" "main" {
  domain = var.domain
}

resource "linode_domain_record" "hub" {
  domain_id   = data.linode_domain.main.id
  name        = "latency"
  record_type = "A"
  target      = linode_instance.hub.ip_address
  ttl_sec     = 300
}
