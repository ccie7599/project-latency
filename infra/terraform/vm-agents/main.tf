variable "linode_token" {
  type      = string
  sensitive = true
}

variable "ssh_public_key" {
  type    = string
  default = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGU4AsrsDExKPBBZGWywY+2BsUw12lkHfEwf/bY03BAp brian@apley.net"
}

variable "hub_ip" {
  type = string
}

variable "admin_ips" {
  type    = list(string)
  default = ["47.224.104.170/32", "172.234.206.146/32"]
}

variable "agent_binary_path" {
  type    = string
  default = "../../../bin/agent-linux"
}

locals {
  distributed_regions = {
    "us-den-1"  = { label = "probe-den",  short = "den" }
    "us-hou-1"  = { label = "probe-hou",  short = "hou" }
    "mx-qro-1"  = { label = "probe-qro",  short = "qro" }
    "co-bog-1"  = { label = "probe-bog1", short = "bog1" }
    "co-bog-2"  = { label = "probe-bog2", short = "bog2" }
    "cl-scl-1"  = { label = "probe-scl",  short = "scl" }
    "de-ber-1"  = { label = "probe-ber",  short = "ber" }
    "de-ham-1"  = { label = "probe-ham",  short = "ham" }
    "fr-mrs-2"  = { label = "probe-mrs",  short = "mrs" }
    "my-kul-1"  = { label = "probe-kul",  short = "kul" }
    "nz-akl-1"  = { label = "probe-akl",  short = "akl" }
    "za-jnb-1"  = { label = "probe-jnb",  short = "jnb" }
  }

  # Core regions restricted for LKE — deploy as VMs instead
  restricted_core_regions = {
    "fr-par"   = { label = "probe-par",  short = "par" }
    "us-iad"   = { label = "probe-iad",  short = "iad" }
    "eu-west"  = { label = "probe-lon",  short = "lon" }
    "es-mad"   = { label = "probe-mad",  short = "mad" }
    "au-mel"   = { label = "probe-mel",  short = "mel" }
  }

  all_vm_regions = merge(local.distributed_regions, local.restricted_core_regions)
  distributed_set = toset(keys(local.distributed_regions))
  common_tags = ["project:latency", "owner:brian"]
}

resource "linode_instance" "agent" {
  for_each = local.all_vm_regions

  label     = each.value.label
  region    = each.key
  type      = contains(local.distributed_set, each.key) ? "g6-dedicated-edge-2" : "g6-nanode-1"
  image     = "linode/ubuntu24.04"
  tags      = local.common_tags
  root_pass = null

  authorized_keys = [var.ssh_public_key]

  lifecycle {
    ignore_changes = [authorized_keys, image]
  }

  provisioner "file" {
    source      = var.agent_binary_path
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
      "mkdir -p /etc/latency",
      "cat > /etc/systemd/system/latency-agent.service << 'UNIT'",
      "[Unit]",
      "Description=Linode Latency Probe Agent",
      "After=network.target",
      "[Service]",
      "Type=simple",
      "ExecStart=/usr/local/bin/latency-agent",
      "Restart=always",
      "RestartSec=5",
      "Environment=REGION=${each.key}",
      "Environment=NATS_SEED_ROUTES=nats-route://${var.hub_ip}:6222",
      "Environment=LISTEN_ADDR=:443",
      "Environment=TLS_CERT=/etc/latency/fullchain.pem",
      "Environment=TLS_KEY=/etc/latency/privkey.pem",
      "[Install]",
      "WantedBy=multi-user.target",
      "UNIT",
      "systemctl daemon-reload",
    ]
    connection {
      type        = "ssh"
      host        = self.ip_address
      user        = "root"
      private_key = file("~/.ssh/id_ed25519")
    }
  }
}

resource "linode_firewall" "vm_agents" {
  label = "latency-vm-agents-fw"
  tags  = local.common_tags

  inbound {
    label    = "nats-cluster"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "6222"
    ipv4     = ["0.0.0.0/0"]
    ipv6     = ["::/0"]
  }

  inbound {
    label    = "nats-monitor"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8222"
    ipv4     = concat(["${var.hub_ip}/32"], var.admin_ips)
  }

  inbound {
    label    = "https-ping"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "443"
    ipv4     = concat(["${var.hub_ip}/32"], var.admin_ips)
  }

  inbound {
    label    = "ssh"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "22"
    ipv4     = var.admin_ips
  }

  inbound_policy  = "DROP"
  outbound_policy = "ACCEPT"

  linodes = [for inst in linode_instance.agent : inst.id]
}

output "agent_ips" {
  value = { for k, v in linode_instance.agent : k => v.ip_address }
}

output "agent_count" {
  value = length(linode_instance.agent)
}
