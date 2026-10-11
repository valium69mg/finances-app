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

# Access to the data bucket without keys (instance profile, reached by the containers
# through IMDSv2 with hop limit 2).
#  - documents (invoices/, tax-filings/): read, write, delete (delete only adds a delete
#    marker: DeleteObjectVersion is not granted, so the previous version stays).
#  - backups/: PutObject only. The role can neither read nor delete backups, so a
#    compromised VM cannot destroy them; the explicit Deny below survives any later
#    broader Allow. Restores use the owner's admin credentials from a laptop.
data "aws_iam_policy_document" "data_bucket_access" {
  statement {
    sid     = "DocumentObjects"
    actions = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = [
      "${aws_s3_bucket.data.arn}/invoices/*",
      "${aws_s3_bucket.data.arn}/tax-filings/*",
    ]
  }

  statement {
    sid       = "ListDocumentPrefixes"
    actions   = ["s3:ListBucket"]
    resources = [aws_s3_bucket.data.arn]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["invoices/*", "tax-filings/*"]
    }
  }

  # HeadBucket (the API's startup check) carries no prefix, so the statement above
  # cannot match it. A prefix-less list would only reveal object key names.
  statement {
    sid       = "HeadBucket"
    actions   = ["s3:ListBucket"]
    resources = [aws_s3_bucket.data.arn]

    condition {
      test     = "Null"
      variable = "s3:prefix"
      values   = ["true"]
    }
  }

  statement {
    sid = "WriteBackupsOnly"
    actions = [
      "s3:PutObject",
      "s3:AbortMultipartUpload",
    ]
    resources = ["${aws_s3_bucket.data.arn}/backups/*"]
  }

  statement {
    sid    = "DenyReadAndDeleteBackups"
    effect = "Deny"
    actions = [
      "s3:GetObject",
      "s3:GetObjectVersion",
      "s3:DeleteObject",
      "s3:DeleteObjectVersion",
    ]
    resources = ["${aws_s3_bucket.data.arn}/backups/*"]
  }
}

resource "aws_iam_role_policy" "data_bucket_access" {
  name   = "data-bucket-access"
  role   = aws_iam_role.app.id
  policy = data.aws_iam_policy_document.data_bucket_access.json
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
