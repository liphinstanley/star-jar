output "urls" {
  description = "Web address of each family's star-jar"
  value       = { for k, m in module.family : k => m.url }
}

output "image_repo" {
  description = "Where to push images"
  value       = "${var.region}-docker.pkg.dev/${var.project}/${google_artifact_registry_repository.images.repository_id}"
}
