output "url" {
  value = google_cloud_run_v2_service.app.uri
}

output "secrets" {
  description = "Secret names that need a value added by hand"
  value       = { for k, s in google_secret_manager_secret.app : k => s.secret_id }
}

output "service_account" {
  description = "Runtime service account (CI needs to act as it to deploy)"
  value       = google_service_account.app.name
}
