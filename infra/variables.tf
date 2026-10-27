variable "project" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "Region for Cloud Run, the database bucket, secrets and images"
  type        = string
  default     = "europe-west1"
}

variable "billing_account" {
  description = "Billing account ID the budget watches"
  type        = string
}

variable "image_tag" {
  description = "Tag of the star-jar image in Artifact Registry (we use the git commit)"
  type        = string
}

variable "families" {
  description = "One star-jar instance per family; the key becomes part of resource names"
  type = map(object({
    timezone = optional(string, "Europe/Berlin")
  }))

  validation {
    condition     = alltrue([for k in keys(var.families) : can(regex("^[a-z][a-z0-9-]{1,14}$", k))])
    error_message = "Family keys must be 2-15 characters: lowercase letters, digits and dashes, starting with a letter."
  }
}
