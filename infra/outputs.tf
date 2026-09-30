output "elastic_ip" {
  description = "Public address of the VM (the DNS record points here)."
  value       = aws_eip.app.public_ip
}

output "instance_id" {
  description = "EC2 instance id."
  value       = aws_instance.app.id
}

output "hostname" {
  description = "Public host name served through Cloudflare."
  value       = var.hostname
}

output "data_volume_id" {
  description = "EBS volume holding /srv/finances (Docker data)."
  value       = aws_ebs_volume.data.id
}

output "ssh_command" {
  description = "SSH hint (empty when SSH is closed). Replace the key path if you moved it."
  value       = local.ssh_enabled ? "ssh -i ${trimsuffix(var.ssh_public_key_path, ".pub")} ubuntu@${aws_eip.app.public_ip}" : "SSH is closed (no ingress rule on port 22)"
}

output "ssh_allowed_cidr" {
  description = "CIDR allowed on port 22 (empty when closed)."
  value       = local.admin_cidr
}
