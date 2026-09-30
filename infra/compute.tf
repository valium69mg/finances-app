data "aws_ssm_parameter" "ubuntu_ami" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/arm64/hvm/ebs-gp3/ami-id"
}

locals {
  subnet_id = sort(data.aws_subnets.default.ids)[0]
}

data "aws_subnet" "selected" {
  id = local.subnet_id
}

resource "aws_key_pair" "admin" {
  key_name   = "finances-prod"
  public_key = file(var.ssh_public_key_path)
}

resource "aws_instance" "app" {
  ami                    = data.aws_ssm_parameter.ubuntu_ami.insecure_value
  instance_type          = var.instance_type
  subnet_id              = local.subnet_id
  vpc_security_group_ids = [aws_security_group.web.id]
  key_name               = aws_key_pair.admin.key_name
  iam_instance_profile   = aws_iam_instance_profile.app.name

  # Pure bootstrap, no secrets: editing it must never replace the VM.
  user_data_replace_on_change = false
  user_data = templatefile("${path.module}/templates/cloud-init.yaml.tftpl", {
    region           = var.region
    data_volume_id   = replace(aws_ebs_volume.data.id, "-", "")
    ssm_cert_name    = aws_ssm_parameter.origin_cert.name
    ssm_key_name     = aws_ssm_parameter.origin_key.name
    repo_url         = var.repo_url
    repo_ref         = var.repo_ref
    docker_data_root = "/srv/finances/docker"
  })

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_size_gb
    encrypted             = true
    delete_on_termination = true
  }

  tags = {
    Name = "finances-prod"
  }

  lifecycle {
    # A newer Canonical AMI must never replace the instance (and its bootstrap).
    ignore_changes = [ami]
  }
}

resource "aws_eip" "app" {
  domain = "vpc"

  tags = {
    Name = "finances-prod"
  }
}

resource "aws_eip_association" "app" {
  instance_id   = aws_instance.app.id
  allocation_id = aws_eip.app.id
}

# Holds /srv/finances: Docker data-root (postgres-data, minio-data volumes). It outlives
# the instance, so it is protected from destroy.
resource "aws_ebs_volume" "data" {
  availability_zone = data.aws_subnet.selected.availability_zone
  type              = "gp3"
  size              = var.data_volume_size_gb
  encrypted         = true

  tags = {
    Name   = "finances-prod-data"
    Backup = "daily"
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_volume_attachment" "data" {
  device_name                    = "/dev/sdf"
  volume_id                      = aws_ebs_volume.data.id
  instance_id                    = aws_instance.app.id
  stop_instance_before_detaching = true
}
