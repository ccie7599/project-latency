output "hub_ip" {
  value = linode_instance.hub.ip_address
}

output "hub_url" {
  value = "http://latency.${var.domain}"
}

output "agent_ips" {
  value = {
    for region, mod in module.agent : region => mod.ip_address
  }
}

output "agent_count" {
  value = length(module.agent)
}

output "dns_records_needed" {
  description = "DNS A records to create in Akamai Edge DNS"
  value = merge(
    { "latency.${var.domain}" = linode_instance.hub.ip_address },
    {
      for region, mod in module.agent :
      "${lookup(local.all_regions[region], "short", region)}.latency.${var.domain}" => mod.ip_address
      if contains(keys(local.all_regions), region)
    }
  )
}

output "cost_estimate" {
  value = {
    hub         = "$24/mo (g6-standard-2)"
    core_agents = "${length([for r in keys(local.agent_regions) : r if !contains(local.distributed_regions, r)])} × $5/mo (g6-nanode-1)"
    edge_agents = "${length([for r in keys(local.agent_regions) : r if contains(local.distributed_regions, r)])} × $43/mo (g6-dedicated-edge-2)"
  }
}
