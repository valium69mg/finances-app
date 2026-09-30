# Cloudflare Origin CA certificate for the host name. The VM downloads both parameters at
# first boot (see templates/cloud-init.yaml.tftpl), so the private key never travels in
# user_data. NOTE: the key is also stored in the (encrypted) Terraform state.

resource "tls_private_key" "origin" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_cert_request" "origin" {
  private_key_pem = tls_private_key.origin.private_key_pem
  dns_names       = [var.hostname]

  subject {
    common_name = var.hostname
  }
}

resource "cloudflare_origin_ca_certificate" "origin" {
  csr                = tls_cert_request.origin.cert_request_pem
  hostnames          = [var.hostname]
  request_type       = "origin-rsa"
  requested_validity = 5475 # 15 years
}

# Standard tier SecureString, encrypted with the AWS-managed key alias/aws/ssm.
resource "aws_ssm_parameter" "origin_cert" {
  name        = "/finances/prod/origin-cert"
  description = "Cloudflare Origin CA certificate (PEM)"
  type        = "SecureString"
  tier        = "Standard"
  value       = cloudflare_origin_ca_certificate.origin.certificate
}

resource "aws_ssm_parameter" "origin_key" {
  name        = "/finances/prod/origin-key"
  description = "Origin certificate private key (PEM)"
  type        = "SecureString"
  tier        = "Standard"
  value       = tls_private_key.origin.private_key_pem
}
