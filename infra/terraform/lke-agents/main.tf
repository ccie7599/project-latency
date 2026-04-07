locals {
  # All 32 core regions in the presales account (excluding us-ord which is the hub)
  regions = {
    "ap-northeast"  = { short = "tyo2" }
    "ap-south"      = { short = "sin" }
    "ap-southeast"  = { short = "syd" }
    "ap-west"       = { short = "bom" }
    # "au-mel" excluded — region restricted for LKE clusters
    "br-gru"        = { short = "gru" }
    "ca-central"    = { short = "yyz" }
    "de-fra-2"      = { short = "fra2" }
    # "es-mad" excluded — region restricted for LKE clusters
    # "au-mel" excluded — region restricted for LKE clusters
    "eu-central"    = { short = "fra" }
    # "eu-west" excluded — region currently restricted for new clusters
    # "fr-par" excluded — region currently restricted for new clusters
    "fr-par-2"      = { short = "par2" }
    "gb-lon"        = { short = "lon2" }
    "id-cgk"        = { short = "cgk" }
    "in-bom-2"      = { short = "bom2" }
    "in-maa"        = { short = "maa" }
    "it-mil"        = { short = "mil" }
    "jp-osa"        = { short = "osa" }
    "jp-tyo-3"      = { short = "tyo3" }
    "nl-ams"        = { short = "ams" }
    "se-sto"        = { short = "sto" }
    "sg-sin-2"      = { short = "sin2" }
    "us-central"    = { short = "dfw" }
    "us-east"       = { short = "ewr" }
    # "us-iad" excluded — region currently restricted for new clusters
    "us-lax"        = { short = "lax" }
    "us-mia"        = { short = "mia" }
    "us-sea"        = { short = "sea" }
    "us-southeast"  = { short = "atl" }
    "us-west"       = { short = "fmt" }
  }
  # us-ord excluded — hub runs in the central landing zone LKE cluster

  common_tags = ["project:latency", "owner:brian"]
}

# One LKE cluster per region with a single g6-standard-1 node
resource "linode_lke_cluster" "agent" {
  for_each = local.regions

  label       = "latency-${each.value.short}"
  region      = each.key
  k8s_version = var.k8s_version
  tags        = local.common_tags

  pool {
    type  = "g6-standard-1"
    count = 1
  }

  lifecycle {
    ignore_changes = [pool[0].count]
  }
}

# Single shared firewall for all LKE agent nodes
resource "linode_firewall" "agents" {
  label = "latency-agents-fw"
  tags  = local.common_tags

  # NATS cluster mesh — open to all (agents discover each other dynamically)
  # Will lock down to cluster IPs after all agents are deployed
  inbound {
    label    = "nats-cluster"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "6222"
    ipv4     = ["0.0.0.0/0"]
    ipv6     = ["::/0"]
  }

  # NATS monitoring — hub + admin only
  inbound {
    label    = "nats-monitor"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "8222"
    ipv4     = concat(["${var.hub_ip}/32"], var.admin_ips)
  }

  # HTTPS /ping — hub + admin only
  inbound {
    label    = "https-ping"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "443"
    ipv4     = concat(["${var.hub_ip}/32"], var.admin_ips)
  }

  # Kubelet health checks (LKE internal)
  inbound {
    label    = "kubelet"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "10250,10256"
    ipv4     = ["192.168.128.0/17"]
  }

  # SSH — admin only
  inbound {
    label    = "ssh"
    action   = "ACCEPT"
    protocol = "TCP"
    ports    = "22"
    ipv4     = var.admin_ips
  }

  inbound_policy  = "DROP"
  outbound_policy = "ACCEPT"

  linodes = flatten([
    for cluster in linode_lke_cluster.agent : [
      for pool in cluster.pool : [
        for node in pool.nodes : node.instance_id
      ]
    ]
  ])
}

output "cluster_ids" {
  value = { for k, v in linode_lke_cluster.agent : k => v.id }
}

output "cluster_endpoints" {
  value = { for k, v in linode_lke_cluster.agent : k => v.api_endpoints }
}
