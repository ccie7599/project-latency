variable "linode_token" {
  type      = string
  sensitive = true
}

variable "k8s_version" {
  type    = string
  default = "1.35"
}

variable "admin_ips" {
  description = "Admin IPs for SSH/HTTPS direct access (set via terraform.tfvars)"
  type        = list(string)
}

variable "hub_ip" {
  description = "Hub node external IP for NATS seed and firewall rules"
  type        = string
}
