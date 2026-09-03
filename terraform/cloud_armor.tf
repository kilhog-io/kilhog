resource "google_compute_region_security_policy" "kilhog" {
  name        = var.security_policy_name
  project     = var.project_id
  region      = var.region
  description = "kilhog WAF policy"

  type = "CLOUD_ARMOR"

  # Parse application/json so WAF signatures inspect field values, not raw
  # object syntax (otherwise CRS 942200 false-positives on subnet create).
  advanced_options_config {
    json_parsing = "STANDARD"
    log_level    = "VERBOSE"
  }

  # Sensitivity 1 = CRS paranoia level 1 only. Default sensitivity 4 includes
  # 942432 (PL4: two special characters in an argument), which matches
  # hyphenated subnet names such as "prod-databases-east1".
  #
  # IPAM JSON fields are also excluded from inspection: names and descriptions
  # routinely contain hyphens and words like "database"; address/prefix look
  # like dotted numbers. Headers, cookies, and the URL stay covered.
  rules {
    action      = "deny(403)"
    priority    = "1000"
    description = "OWASP SQLi (sensitivity 1; IPAM JSON fields excluded)"

    match {
      expr {
        expression = "evaluatePreconfiguredWaf('sqli-v33-stable', {'sensitivity': 1})"
      }
    }

    preconfigured_waf_config {
      exclusion {
        target_rule_set = "sqli-v33-stable"

        request_query_param {
          operator = "EQUALS"
          value    = "name"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "description"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "address"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "prefix"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "type"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "tags"
        }
      }
    }
  }

  rules {
    action      = "deny(403)"
    priority    = "1001"
    description = "OWASP XSS (free-text IPAM fields excluded)"

    match {
      expr {
        expression = "evaluatePreconfiguredWaf('xss-v33-stable', {'sensitivity': 1})"
      }
    }

    preconfigured_waf_config {
      exclusion {
        target_rule_set = "xss-v33-stable"

        request_query_param {
          operator = "EQUALS"
          value    = "name"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "description"
        }
        request_query_param {
          operator = "EQUALS"
          value    = "tags"
        }
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
