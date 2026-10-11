# Private S3 bucket for the CFDI/acuse documents and the nightly Postgres backups.
#
# Key layout (the application writes the documents, the VM backup timer writes backups):
#   invoices/{invoice id}/{kind}-{random}.{ext}          CFDI XML and PDF
#   tax-filings/{period}/{kind}/{random}.{ext}           SAT acuse and payment comprobante
#   backups/daily/YYYY-MM-DD.dump                        nightly pg_dump -Fc
#   backups/monthly/YYYY-MM.dump                          copy taken on day 1 of each month
#
# The bucket name is generated (bucket_prefix) and exposed as an output: it is not
# written in the repository.
resource "aws_s3_bucket" "data" {
  bucket_prefix = "finances-prod-"
  force_destroy = false

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_public_access_block" "data" {
  bucket = aws_s3_bucket.data.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "data" {
  bucket = aws_s3_bucket.data.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "data" {
  bucket = aws_s3_bucket.data.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# A deleted or replaced document stays recoverable as a noncurrent version (see the
# lifecycle rules). The VM role cannot delete versions, only add delete markers.
resource "aws_s3_bucket_versioning" "data" {
  bucket = aws_s3_bucket.data.id

  versioning_configuration {
    status = "Enabled"
  }
}

data "aws_iam_policy_document" "data_bucket" {
  statement {
    sid     = "DenyInsecureTransport"
    effect  = "Deny"
    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.data.arn,
      "${aws_s3_bucket.data.arn}/*",
    ]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "data" {
  bucket = aws_s3_bucket.data.id
  policy = data.aws_iam_policy_document.data_bucket.json

  depends_on = [aws_s3_bucket_public_access_block.data]
}

resource "aws_s3_bucket_lifecycle_configuration" "data" {
  bucket = aws_s3_bucket.data.id

  # Needs versioning in place first (noncurrent version rules).
  depends_on = [aws_s3_bucket_versioning.data]

  rule {
    id     = "abort-incomplete-multipart"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  rule {
    id     = "backups-daily-expire"
    status = "Enabled"

    filter {
      prefix = "backups/daily/"
    }

    expiration {
      days = 30
    }
  }

  rule {
    id     = "backups-monthly-expire"
    status = "Enabled"

    filter {
      prefix = "backups/monthly/"
    }

    expiration {
      days = 365
    }
  }

  # Backups are never deleted by hand (the VM role cannot), so every noncurrent version
  # under backups/ is just the leftover of an expiry or an overwrite: drop it after a day
  # and remove the orphaned delete markers.
  rule {
    id     = "backups-noncurrent-expire"
    status = "Enabled"

    filter {
      prefix = "backups/"
    }

    noncurrent_version_expiration {
      noncurrent_days = 1
    }

    expiration {
      expired_object_delete_marker = true
    }
  }

  # Documents: a deleted or replaced CFDI/acuse is recoverable for a year.
  rule {
    id     = "invoices-noncurrent-expire"
    status = "Enabled"

    filter {
      prefix = "invoices/"
    }

    noncurrent_version_expiration {
      noncurrent_days = 365
    }

    expiration {
      expired_object_delete_marker = true
    }
  }

  rule {
    id     = "tax-filings-noncurrent-expire"
    status = "Enabled"

    filter {
      prefix = "tax-filings/"
    }

    noncurrent_version_expiration {
      noncurrent_days = 365
    }

    expiration {
      expired_object_delete_marker = true
    }
  }
}
