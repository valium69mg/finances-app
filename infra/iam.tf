data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "app" {
  name               = "finances-prod-instance"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

# The instance can do exactly one thing: read the two certificate parameters.
data "aws_iam_policy_document" "read_origin_cert" {
  statement {
    sid     = "ReadOriginCertParameters"
    actions = ["ssm:GetParameter"]
    resources = [
      aws_ssm_parameter.origin_cert.arn,
      aws_ssm_parameter.origin_key.arn,
    ]
  }

  # Decryption with the AWS-managed SSM key (alias/aws/ssm). Its key id is not known until
  # the first SecureString exists, so the key is matched by the ViaService condition: the
  # role can only decrypt through SSM in this region.
  statement {
    sid       = "DecryptWithAwsManagedSsmKey"
    actions   = ["kms:Decrypt"]
    resources = ["arn:aws:kms:${var.region}:${data.aws_caller_identity.current.account_id}:key/*"]

    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["ssm.${var.region}.amazonaws.com"]
    }
  }
}

data "aws_caller_identity" "current" {}

resource "aws_iam_role_policy" "read_origin_cert" {
  name   = "read-origin-cert"
  role   = aws_iam_role.app.id
  policy = data.aws_iam_policy_document.read_origin_cert.json
}

resource "aws_iam_instance_profile" "app" {
  name = "finances-prod-instance"
  role = aws_iam_role.app.name
}

# Role used by Data Lifecycle Manager to snapshot the data volume (per AWS docs).
data "aws_iam_policy_document" "dlm_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["dlm.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dlm" {
  name               = "finances-prod-dlm"
  assume_role_policy = data.aws_iam_policy_document.dlm_assume.json
}

data "aws_iam_policy_document" "dlm" {
  statement {
    actions = [
      "ec2:CreateSnapshot",
      "ec2:CreateSnapshots",
      "ec2:DeleteSnapshot",
      "ec2:DescribeInstances",
      "ec2:DescribeVolumes",
      "ec2:DescribeSnapshots",
    ]
    resources = ["*"]
  }

  statement {
    actions   = ["ec2:CreateTags"]
    resources = ["arn:aws:ec2:*::snapshot/*"]
  }
}

resource "aws_iam_role_policy" "dlm" {
  name   = "snapshot-data-volume"
  role   = aws_iam_role.dlm.id
  policy = data.aws_iam_policy_document.dlm.json
}
