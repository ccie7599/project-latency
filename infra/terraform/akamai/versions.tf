terraform {
  required_version = ">= 1.5.0"

  required_providers {
    akamai = {
      source  = "akamai/akamai"
      version = "~> 6.0"
    }
  }
}

provider "akamai" {
  edgerc         = "~/.edgerc"
  config_section = "default"
}
