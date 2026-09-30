data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }

  filter {
    name   = "default-for-az"
    values = ["true"]
  }
}

data "cloudflare_ip_ranges" "this" {}

# Auto-detect the caller's public IPv4 only when SSH is enabled and no CIDR was given.
data "http" "my_ip" {
  count = var.enable_ssh && var.admin_cidr == null ? 1 : 0

  url = "https://checkip.amazonaws.com"
}

locals {
  cloudflare_cidrs_v4 = toset(data.cloudflare_ip_ranges.this.ipv4_cidrs)
  cloudflare_cidrs_v6 = toset(data.cloudflare_ip_ranges.this.ipv6_cidrs)

  admin_cidr = !var.enable_ssh ? "" : (
    var.admin_cidr == null ? "${chomp(data.http.my_ip[0].response_body)}/32" : var.admin_cidr
  )
  ssh_enabled = local.admin_cidr != ""
}

resource "aws_security_group" "web" {
  name        = "finances-prod"
  description = "HTTPS from Cloudflare only, SSH from the admin address only"
  vpc_id      = data.aws_vpc.default.id

  tags = {
    Name = "finances-prod"
  }
}

# One rule per Cloudflare range (about 22 in total, well under the 60 rules per group).
resource "aws_vpc_security_group_ingress_rule" "https_v4" {
  for_each = local.cloudflare_cidrs_v4

  security_group_id = aws_security_group.web.id
  description       = "HTTPS from Cloudflare"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = each.value
}

resource "aws_vpc_security_group_ingress_rule" "https_v6" {
  for_each = local.cloudflare_cidrs_v6

  security_group_id = aws_security_group.web.id
  description       = "HTTPS from Cloudflare"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv6         = each.value
}

resource "aws_vpc_security_group_ingress_rule" "ssh" {
  count = local.ssh_enabled ? 1 : 0

  security_group_id = aws_security_group.web.id
  description       = "SSH from the admin address"
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
  cidr_ipv4         = local.admin_cidr
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.web.id
  description       = "All outbound traffic"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "all_v6" {
  security_group_id = aws_security_group.web.id
  description       = "All outbound traffic (IPv6)"
  ip_protocol       = "-1"
  cidr_ipv6         = "::/0"
}
