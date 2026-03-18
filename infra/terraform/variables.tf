variable "linode_token" {
  description = "Linode API token"
  type        = string
  sensitive   = true
}

variable "ssh_public_key" {
  description = "SSH public key for instance access"
  type        = string
}

variable "admin_ip" {
  description = "Admin IP for SSH access (CIDR)"
  type        = string
}

variable "domain" {
  description = "Domain for DNS records"
  type        = string
  default     = "connected-cloud.io"
}

variable "hub_region" {
  description = "Region for the hub server"
  type        = string
  default     = "us-ord"
}

variable "auth_token" {
  description = "API auth token for protected endpoints"
  type        = string
  sensitive   = true
}

variable "agent_binary_path" {
  description = "Path to the cross-compiled agent binary"
  type        = string
  default     = "../../bin/agent-linux"
}

variable "hub_binary_path" {
  description = "Path to the hub binary"
  type        = string
  default     = "../../bin/hub"
}

variable "agent_regions" {
  description = "Map of region IDs to deploy agents to. Set to null to deploy to all regions."
  type        = map(object({
    label = string
  }))
  default = null
}
