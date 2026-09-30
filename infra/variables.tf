variable "region" {
  description = "AWS region."
  type        = string
  default     = "us-east-2"
}

variable "zone_name" {
  description = "Cloudflare zone (registered domain), e.g. example.com."
  type        = string
}

variable "hostname" {
  description = "Public host name served by the stack, inside the zone, e.g. app.example.com."
  type        = string
}

variable "budget_email" {
  description = "Email that receives the AWS budget alerts."
  type        = string
}

variable "budget_limit_usd" {
  description = "Monthly AWS cost budget in USD."
  type        = number
  default     = 30
}

variable "instance_type" {
  description = "EC2 instance type (must be arm64: the AMI is Ubuntu arm64)."
  type        = string
  default     = "t4g.small"
}

variable "root_volume_size_gb" {
  description = "Root volume size in GB (gp3, encrypted)."
  type        = number
  default     = 12
}

variable "data_volume_size_gb" {
  description = "Data volume size in GB (gp3, encrypted, holds /srv/finances and Docker data)."
  type        = number
  default     = 50
}

variable "ssh_public_key_path" {
  description = "Path to the SSH public key registered on the instance (the private key never enters Terraform)."
  type        = string
  default     = "./.keys/finances-prod.pub"
}

variable "enable_ssh" {
  description = "Open port 22. Set to false (or admin_cidr to \"\") to remove the SSH rule entirely."
  type        = bool
  default     = true
}

variable "admin_cidr" {
  description = "CIDR allowed to reach SSH. null (default) auto-detects the caller's public IPv4 as a /32 on every run via checkip.amazonaws.com; an explicit empty string removes the SSH rule."
  type        = string
  default     = null
}

variable "repo_url" {
  description = "Git repository cloned on the VM (public)."
  type        = string
  default     = "https://github.com/valium69mg/finances-app.git"
}

variable "repo_ref" {
  description = "Branch, tag or commit checked out on the VM."
  type        = string
  default     = "main"
}

variable "snapshot_time_utc" {
  description = "Daily DLM snapshot time (HH:MM, UTC)."
  type        = string
  default     = "06:00"
}

variable "snapshot_retention" {
  description = "Number of daily snapshots of the data volume to keep."
  type        = number
  default     = 7
}
