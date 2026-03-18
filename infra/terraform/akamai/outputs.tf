output "property_id" {
  value = akamai_property.latency.id
}

output "cp_code_id" {
  value = akamai_cp_code.latency.id
}

output "edge_hostname" {
  value = var.edge_hostname
}

output "property_url" {
  value = "https://${var.property_hostname}"
}

# output "ds2_stream_id" {
#   value = akamai_datastream.latency.id
# }
