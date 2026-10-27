module "family" {
  source   = "./modules/family"
  for_each = var.families

  name     = each.key
  project  = var.project
  region   = var.region
  image    = "${var.region}-docker.pkg.dev/${var.project}/${google_artifact_registry_repository.images.repository_id}/star-jar:${var.image_tag}"
  timezone = each.value.timezone

  depends_on = [google_project_service.apis]
}
