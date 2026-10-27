variable "name" {
  description = "Short family key, e.g. home"
  type        = string
}

variable "project" {
  type = string
}

variable "region" {
  type = string
}

variable "image" {
  description = "Full image reference to run"
  type        = string
}

variable "timezone" {
  description = "Tasks reset at midnight in this time zone"
  type        = string
}
