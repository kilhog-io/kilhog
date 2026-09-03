resource "google_compute_region_security_policy" "kilhog" {
  name        = var.security_policy_name
  project     = var.project_id
  region      = var.region
  description = "kilhog WAF policy"

  type = "CLOUD_ARMOR"

  advanced_options_config {
    json_parsing = "STANDARD"
    log_level    = "VERBOSE"
  }

  rules {
    action      = "deny(403)"
    priority    = "1000"
    description = "OWASP SQLi (942200 excluded — false positive on IPAM JSON)"

    match {
      expr {
        expression = "evaluatePreconfiguredWaf('sqli-v33-stable', {'opt_out_rule_ids': ['owasp-crs-v030301-id942200-sqli']})"
      }
    }
  }

  rules {
    action      = "deny(403)"
    priority    = "1001"
    description = "OWASP XSS"

    match {
      expr {
        expression = "evaluatePreconfiguredWaf('xss-v33-stable')"
      }
    }
  }

  rules {
    action      = "allow"
    priority    = "2147483647"
    description = "Default allow"

    match {
      versioned_expr = "SRC_IPS_V1"

      config {
        src_ip_ranges = ["*"]
      }
    }
  }
}
