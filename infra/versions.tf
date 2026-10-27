terraform {
  required_version = ">= 1.11" # ephemeral values and write-only attributes

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.8"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }

  # created once by hand, see README: gcloud storage buckets create gs://star-jar-liphin-tfstate ...
  backend "gcs" {
    bucket = "star-jar-liphin-tfstate"
    prefix = "main"
  }
}

provider "google" {
  project = var.project
  region  = var.region

  # bill API calls (e.g. Budgets) to our project, needed with personal user credentials
  user_project_override = true
  billing_project       = var.project
}
