output "hub_ip" {
  value = linode_instance.hub.ip_address
}

output "hub_url" {
  value = "https://latency.${var.domain}"
}

output "agent_ips" {
  value = {
    for region, mod in module.agent : region => mod.ip_address
  }
}

output "agent_count" {
  value = length(module.agent)
}
