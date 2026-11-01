output "urls" {
  description = "Web address of each family's star-jar"
  value       = { for k, m in module.family : k => m.url }
}

output "image_repo" {
  description = "Where to push images"
  value       = "${var.region}-docker.pkg.dev/${var.project}/${google_artifact_registry_repository.images.repository_id}"
}

output "github_wif_provider" {
  description = "For the workflow's workload_identity_provider"
  value       = google_iam_workload_identity_pool_provider.star_jar.name
}

output "github_deployer" {
  description = "For the workflow's service_account"
  value       = google_service_account.deployer.email
}
