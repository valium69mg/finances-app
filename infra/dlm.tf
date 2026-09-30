# Daily snapshot of every volume tagged Backup=daily (the data volume). A minimal safety
# net, not a replacement for real backups.
resource "aws_dlm_lifecycle_policy" "daily" {
  description        = "Daily snapshots of volumes tagged Backup daily"
  execution_role_arn = aws_iam_role.dlm.arn
  state              = "ENABLED"

  policy_details {
    resource_types = ["VOLUME"]

    target_tags = {
      Backup = "daily"
    }

    schedule {
      name      = "daily-${var.snapshot_retention}-kept"
      copy_tags = true

      create_rule {
        interval      = 24
        interval_unit = "HOURS"
        times         = [var.snapshot_time_utc]
      }

      retain_rule {
        count = var.snapshot_retention
      }

      tags_to_add = {
        SnapshotCreator = "dlm"
      }
    }
  }
}
