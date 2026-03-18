variable "region" {
  type = string
}

variable "label" {
  type = string
}

variable "nats_url" {
  type      = string
  sensitive = true
}

variable "binary_path" {
  type = string
}

variable "ssh_key" {
  type = string
}

variable "admin_ip" {
  type = string
}

variable "tags" {
  type    = list(string)
  default = []
}

resource "linode_instance" "agent" {
  label     = var.label
  region    = var.region
  type      = "g6-nanode-1"
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
      "",
      "[Service]",
      "Type=simple",
      "ExecStart=/usr/local/bin/latency-agent",
      "Restart=always",
      "RestartSec=5",
      "Environment=REGION=${var.region}",
      "Environment=NATS_URL=${var.nats_url}",
      "Environment=LISTEN_ADDR=:8080",
      "",
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

  inbound {
    label    = "http-ping"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8080"
    ipv4     = ["0.0.0.0/0"]
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

  linodes = [linode_instance.agent.id]
}

output "ip_address" {
  value = linode_instance.agent.ip_address
}
