# ---------------------------------------------------------------------------
# Akamai CDN Property — latency-demo.connected-cloud.io
# Origin: latency.connected-cloud.io (hub server)
# ---------------------------------------------------------------------------

# CP Code for this property
resource "akamai_cp_code" "latency" {
  name        = "latency-demo"
  contract_id = var.contract_id
  group_id    = var.group_id
  product_id  = "prd_SPM"
}

# Edge hostname — uses shared SAN cert (enrollment 293468)
resource "akamai_edge_hostname" "latency" {
  contract_id   = var.contract_id
  group_id      = var.group_id
  product_id    = "prd_SPM"
  edge_hostname = var.edge_hostname
  ip_behavior   = "IPV4"
  certificate   = 293468
}

# Property rules template
data "akamai_property_rules_template" "latency" {
  template_file = abspath("${path.root}/property-rules.json")

  variables {
    name  = "origin_hostname"
    value = var.origin_hostname
    type  = "string"
  }

  variables {
    name  = "cp_code_id"
    value = akamai_cp_code.latency.id
    type  = "number"
  }

}

# Property
resource "akamai_property" "latency" {
  name        = var.property_hostname
  product_id  = "prd_SPM"
  contract_id = var.contract_id
  group_id    = var.group_id

  hostnames {
    cname_from             = var.property_hostname
    cname_to               = var.edge_hostname
    cert_provisioning_type = "CPS_MANAGED"
  }

  rule_format = "latest"
  rules       = data.akamai_property_rules_template.latency.json
}

# Staging activation
resource "akamai_property_activation" "staging" {
  property_id                    = akamai_property.latency.id
  contact                        = var.notification_emails
  version                        = akamai_property.latency.latest_version
  network                        = "STAGING"
  note                           = "Latency demo — Ion property with WebSocket, DS2 logging"
  auto_acknowledge_rule_warnings = true
}

# Production activation
resource "akamai_property_activation" "production" {
  property_id                    = akamai_property.latency.id
  contact                        = var.notification_emails
  version                        = akamai_property.latency.latest_version
  network                        = "PRODUCTION"
  note                           = "Latency demo — production activation"
  auto_acknowledge_rule_warnings = true

  compliance_record {
    noncompliance_reason_no_production_traffic {
    }
  }

  depends_on = [akamai_property_activation.staging]
}

# ---------------------------------------------------------------------------
# DataStream 2 — Real-time log delivery to shared ClickHouse webhook
# ---------------------------------------------------------------------------

# TODO: DS2 — webhook auth failing validation, needs Vault cred sync
# Uncomment when ds2-im-demo webhook creds are confirmed
resource "akamai_datastream" "latency" {
  active      = true
  stream_name = "latency-demo-ds2"
  contract_id = var.contract_id
  group_id    = var.group_id
  properties  = [akamai_property.latency.id]

  dataset_fields = [
    1000,  # CP code
    1001,  # Request host
    1002,  # Request ID
    1003,  # Request method
    1005,  # Bytes
    1006,  # Client IP
    1008,  # HTTP status code
    1013,  # Request path
    1016,  # Response Content-Type
    1017,  # User-Agent
    1102,  # Turn around time
    2010,  # Cache status
    2012,  # Country/Region
    2014,  # City
    3000,  # EdgeWorkers usage
  ]

  delivery_configuration {
    format = "JSON"
    frequency {
      interval_in_secs = 30
    }
  }

  https_connector {
    display_name        = "latency-demo-ds2-webhook"
    endpoint            = var.ds2_webhook_endpoint
    authentication_type = "BASIC"
    user_name           = var.ds2_webhook_username
    password            = var.ds2_webhook_password
    content_type        = "application/json"
    compress_logs       = false
  }

  notification_emails = var.notification_emails
  depends_on          = [akamai_property_activation.staging]
}

# ---------------------------------------------------------------------------
# DNS CNAME — point property hostname to edge hostname
# ---------------------------------------------------------------------------

resource "akamai_dns_record" "latency_demo_cname" {
  zone       = "connected-cloud.io"
  name       = var.property_hostname
  recordtype = "CNAME"
  ttl        = 300
  target     = [var.edge_hostname]
}
