# GitHub Actions deploys without any stored key: GitHub's signed token for a run on
# liphinstanley/star-jar main is exchanged for a short-lived Google token (Workload Identity Federation)

locals {
  github_repo = "liphinstanley/star-jar"
}

resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "github"
  display_name              = "GitHub Actions"

  depends_on = [google_project_service.apis]
}

resource "google_iam_workload_identity_pool_provider" "star_jar" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "star-jar"
  display_name                       = "star-jar repo"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }
  # only this repo, only the main branch
  attribute_condition = "assertion.repository == '${local.github_repo}' && assertion.ref == 'refs/heads/main'"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "deployer" {
  account_id   = "github-deployer"
  display_name = "GitHub Actions deployer"
}

# runs from the repo may act as the deployer
resource "google_service_account_iam_member" "deployer_github" {
  service_account_id = google_service_account.deployer.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${local.github_repo}"
}

# push images, only to our repository
resource "google_artifact_registry_repository_iam_member" "deployer_push" {
  repository = google_artifact_registry_repository.images.name
  location   = google_artifact_registry_repository.images.location
  role       = "roles/artifactregistry.writer"
  member     = google_service_account.deployer.member
}

# roll out new revisions
resource "google_project_iam_member" "deployer_run" {
  project = var.project
  role    = "roles/run.developer"
  member  = google_service_account.deployer.member
}

# a revision runs as the family's service account, so the deployer must be allowed to "use" it
resource "google_service_account_iam_member" "deployer_act_as" {
  for_each           = module.family
  service_account_id = each.value.service_account
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.deployer.member
}
