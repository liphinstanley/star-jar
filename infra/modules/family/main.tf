locals {
  prefix = "star-jar-${var.name}"

  # env var name => secret name suffix
  secrets = {
    FAMILY_PASSWORD = "family-password"
    PARENT_PIN      = "parent-pin"
    SESSION_KEY     = "session-key"
  }
}

# the app runs as its own identity with only the access it needs
resource "google_service_account" "app" {
  account_id   = "sj-${var.name}"
  display_name = "star-jar ${var.name}"
}

# --- database backup (Litestream) ---

resource "google_storage_bucket" "db" {
  name                        = "${var.project}-${var.name}-db"
  location                    = upper(var.region)
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  # Litestream replaces small files all the time; soft-deleted copies would be billed for 7 days
  soft_delete_policy {
    retention_duration_seconds = 0
  }
}

resource "google_storage_bucket_iam_member" "app" {
  bucket = google_storage_bucket.db.name
  role   = "roles/storage.objectUser"
  member = google_service_account.app.member
}

# --- secrets ---

resource "google_secret_manager_secret" "app" {
  for_each  = local.secrets
  secret_id = "${local.prefix}-${each.value}"

  replication {
    user_managed {
      replicas {
        location = var.region
      }
    }
  }
}

resource "google_secret_manager_secret_iam_member" "app" {
  for_each  = local.secrets
  secret_id = google_secret_manager_secret.app[each.key].id
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.app.member
}

# the session key is random; password and PIN are added by hand so they never touch Terraform.
# ephemeral + write-only: the key goes straight to Secret Manager and is never stored in the state file
ephemeral "random_password" "session_key" {
  length = 64
}

resource "google_secret_manager_secret_version" "session_key" {
  secret                 = google_secret_manager_secret.app["SESSION_KEY"].id
  secret_data_wo         = sha256(ephemeral.random_password.session_key.result) # 64 hex characters, what the app expects
  secret_data_wo_version = 1                                                    # bump to rotate the key (signs out every device)
}

# --- the app ---

resource "google_cloud_run_v2_service" "app" {
  name                = local.prefix
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false

  # CI finds every family's service by this label
  labels = {
    app    = "star-jar"
    family = var.name
  }

  template {
    service_account                  = google_service_account.app.email
    execution_environment            = "EXECUTION_ENVIRONMENT_GEN2"
    max_instance_request_concurrency = 80

    # SQLite has one writer: never run two copies of the app at once
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }

    containers {
      image = var.image

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
        cpu_idle          = true # pay only while handling requests
        startup_cpu_boost = true
      }

      env {
        name  = "TZ"
        value = var.timezone
      }
      env {
        name  = "REPLICA_URL"
        value = "gs://${google_storage_bucket.db.name}/star-jar"
      }

      dynamic "env" {
        for_each = local.secrets
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.app[env.key].secret_id
              version = "latest"
            }
          }
        }
      }

      # restore from the bucket can take a few seconds on a cold start
      startup_probe {
        http_get {
          path = "/healthz"
        }
        period_seconds    = 2
        timeout_seconds   = 1
        failure_threshold = 30
      }
    }
  }

  # CI deploys new images; Terraform owns everything else
  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].containers[0].image,
    ]
  }

  depends_on = [
    google_storage_bucket_iam_member.app,
    google_secret_manager_secret_iam_member.app,
  ]
}

# the app has its own login, so the URL itself is public
resource "google_cloud_run_v2_service_iam_member" "public" {
  name     = google_cloud_run_v2_service.app.name
  location = google_cloud_run_v2_service.app.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}
