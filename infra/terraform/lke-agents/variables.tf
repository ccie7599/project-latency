variable "linode_token" {
  type      = string
  sensitive = true
}

variable "k8s_version" {
  type    = string
  default = "1.35"
}

variable "admin_ips" {
  type    = list(string)
  default = ["47.224.104.170/32", "172.234.206.146/32"]
}

variable "hub_ip" {
  description = "Hub node external IP for NATS seed and firewall rules"
  type        = string
}
