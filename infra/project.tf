locals {
  apis = [
    "artifactregistry.googleapis.com",
    "billingbudgets.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "storage.googleapis.com",
  ]
}

resource "google_project_service" "apis" {
  for_each = toset(local.apis)

  service            = each.key
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "images" {
  repository_id = "star-jar"
  location      = var.region
  format        = "DOCKER"

  # stay inside the free 0.5 GB: keep the 5 newest images, delete anything older than a week
  cleanup_policy_dry_run = false
  cleanup_policies {
    id     = "keep-newest-5"
    action = "KEEP"
    most_recent_versions {
      keep_count = 5
    }
  }
  cleanup_policies {
    id     = "delete-older-than-7d"
    action = "DELETE"
    condition {
      tag_state  = "ANY"
      older_than = "604800s"
    }
  }

  depends_on = [google_project_service.apis]
}

# created with gcloud on 2026-10-26, now managed here
import {
  to = google_billing_budget.monthly
  id = "billingAccounts/01B54D-C8D8B3-850ED5/budgets/dc5e9b5d-2339-44d1-8795-ce71ceeada39"
}

resource "google_billing_budget" "monthly" {
  billing_account = var.billing_account
  display_name    = "personal-eur monthly 5 EUR"

  amount {
    specified_amount {
      currency_code = "EUR"
      units         = "5"
    }
  }

  budget_filter {
    calendar_period        = "MONTH"
    credit_types_treatment = "INCLUDE_ALL_CREDITS"
  }

  threshold_rules {
    threshold_percent = 0.5
  }
  threshold_rules {
    threshold_percent = 0.9
  }
  threshold_rules {
    threshold_percent = 1.0
  }
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "FORECASTED_SPEND"
  }
}
