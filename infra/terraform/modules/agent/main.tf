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

variable "tags" {
  type    = list(string)
  default = []
}

locals {
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

  lifecycle {
    ignore_changes = [authorized_keys, image]
  }

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
      "Environment=LISTEN_ADDR=:443",
      "Environment=TLS_CERT=/etc/latency/fullchain.pem",
      "Environment=TLS_KEY=/etc/latency/privkey.pem",
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

output "ip_address" {
  value = linode_instance.agent.ip_address
}

output "instance_id" {
  value = linode_instance.agent.id
}
