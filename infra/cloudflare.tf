data "cloudflare_zone" "this" {
  filter = {
    name = var.zone_name
  }
}

resource "cloudflare_dns_record" "app" {
  zone_id = data.cloudflare_zone.this.id
  name    = var.hostname
  type    = "A"
  content = aws_eip.app.public_ip
  proxied = true
  ttl     = 1 # automatic (required when proxied)
  comment = "finances production (managed by terraform)"
}

locals {
  zone_settings = {
    ssl              = "strict"
    always_use_https = "on"
    min_tls_version  = "1.2"
  }
}

resource "cloudflare_zone_setting" "this" {
  for_each = local.zone_settings

  zone_id    = data.cloudflare_zone.this.id
  setting_id = each.key
  value      = each.value
}
