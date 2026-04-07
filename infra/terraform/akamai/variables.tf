variable "contract_id" {
  type    = string
  default = "ctr_M-1YX7F61"
}

variable "group_id" {
  type    = string
  default = "grp_203183"
}

variable "origin_hostname" {
  description = "Origin server hostname (hub)"
  type        = string
  default     = "latency-origin.presales.connected-cloud.io"
}

variable "property_hostname" {
  description = "CDN-fronted hostname"
  type        = string
  default     = "latency-demo.connected-cloud.io"
}

variable "edge_hostname" {
  description = "Akamai edge hostname"
  type        = string
  default     = "latency-demo.connected-cloud.io.edgekey.net"
}

variable "notification_emails" {
  type    = list(string)
  default = ["bapley@akamai.com"]
}

variable "ds2_webhook_endpoint" {
  description = "DS2 webhook URL for log delivery"
  type        = string
  default     = "https://ds2-im-demo.connected-cloud.io/api/ds2/webhook"
}

variable "ds2_webhook_username" {
  type      = string
  sensitive = true
}

variable "ds2_webhook_password" {
  type      = string
  sensitive = true
}
