terraform {
  required_providers {
    linode = {
      source = "linode/linode"
    }
  }
}

variable "region" {
  type = string
}

variable "label" {
  type = string
}

variable "is_distributed" {
  description = "Whether this is a distributed/edge region (requires g6-dedicated-edge-2)"
  type        = bool
  default     = false
}

variable "nats_seed_routes" {
  type = string
}

variable "binary_path" {
  type = string
}

variable "ssh_key" {
  type = string
}

variable "hub_ip" {
  description = "Hub IP in CIDR notation for firewall rules"
  type        = string
}

variable "admin_ip" {
  description = "Admin/home IP in CIDR notation for firewall rules"
  type        = string
}

variable "tags" {
  type    = list(string)
  default = []
}

locals {
  # Distributed regions: smallest available is g6-dedicated-edge-2 ($43/mo)
  # Core regions: g6-nanode-1 ($5/mo)
  instance_type = var.is_distributed ? "g6-dedicated-edge-2" : "g6-nanode-1"
}

resource "linode_instance" "agent" {
  label     = var.label
  region    = var.region
  type      = local.instance_type
  image     = "linode/ubuntu24.04"
  tags      = var.tags
  root_pass = null

  authorized_keys = [var.ssh_key]

  provisioner "file" {
    source      = var.binary_path
    destination = "/usr/local/bin/latency-agent"
    connection {
      type        = "ssh"
      host        = self.ip_address
      user        = "root"
      private_key = file("~/.ssh/id_ed25519")
    }
  }

  provisioner "remote-exec" {
    inline = [
      "chmod +x /usr/local/bin/latency-agent",
      "cat > /etc/systemd/system/latency-agent.service << 'UNIT'",
      "[Unit]",
      "Description=Linode Latency Probe Agent",
      "After=network.target",
      "[Service]",
      "Type=simple",
      "ExecStart=/usr/local/bin/latency-agent",
      "Restart=always",
      "RestartSec=5",
      "Environment=REGION=${var.region}",
      "Environment=NATS_SEED_ROUTES=${var.nats_seed_routes}",
      "Environment=LISTEN_ADDR=:8080",
      "[Install]",
      "WantedBy=multi-user.target",
      "UNIT",
      "systemctl daemon-reload",
      "systemctl enable --now latency-agent",
    ]

    connection {
      type        = "ssh"
      host        = self.ip_address
      user        = "root"
      private_key = file("~/.ssh/id_ed25519")
    }
  }
}

resource "linode_firewall" "agent" {
  label = "${var.label}-fw"
  tags  = var.tags

  # HTTP /ping — hub + admin only
  inbound {
    label    = "http-ping-hub"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8080"
    ipv4     = [var.hub_ip, var.admin_ip]
  }

  # NATS cluster routes — all cluster members need to connect
  inbound {
    label    = "nats-cluster"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "6222"
    ipv4     = ["0.0.0.0/0"]
    ipv6     = ["::/0"]
  }

  # NATS monitoring — hub only (for /routez scraping)
  inbound {
    label    = "nats-monitor-hub"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8222"
    ipv4     = [var.hub_ip]
  }

  # SSH — admin only
  inbound {
    label    = "ssh-admin"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "22"
    ipv4     = [var.admin_ip]
  }

  inbound_policy  = "DROP"
  outbound_policy = "ACCEPT"

  linodes = [linode_instance.agent.id]
}

output "ip_address" {
  value = linode_instance.agent.ip_address
}
