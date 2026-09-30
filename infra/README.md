# Production infrastructure (Terraform)

One EC2 VM (Ubuntu 24.04 arm64) in AWS `us-east-2`, behind Cloudflare (orange cloud, Full strict).
The stack itself is `deploy/docker-compose.prod.yml` (runbook: `deploy/README.md`); this
code only provides what the VM needs: Docker, swap, a separate data volume, the Cloudflare
origin certificate and the repository clone. It does **not** create `deploy/.env.prod` and
does **not** start the stack.

| Piece | Details |
| --- | --- |
| Network | default VPC and subnet; security group: 443 only from Cloudflare ranges (one rule per CIDR), 22 only from `admin_cidr`, port 80 closed, egress open |
| Compute | `t4g.small`, root gp3 12 GB encrypted, IMDSv2 required, Elastic IP; a new Canonical AMI never replaces the instance (`ignore_changes = [ami]`) |
| Data | gp3 50 GB encrypted EBS (`prevent_destroy`), mounted at `/srv/finances`, Docker `data-root` = `/srv/finances/docker` |
| Certificate | Cloudflare Origin CA (RSA 2048, 15 years) stored as SSM SecureString; the VM downloads it at first boot to `/etc/finances/certs` |
| IAM | instance role can only `ssm:GetParameter` on the two certificate parameters; DLM role for snapshots |
| Snapshots | DLM, daily at `snapshot_time_utc`, keeps 7, volumes tagged `Backup=daily` |
| Cloudflare | proxied `A` record, SSL `strict`, Always Use HTTPS, minimum TLS 1.2 |
| Budget | monthly cost budget, alert at 80% actual and 100% forecast |

## Prerequisites

- Terraform >= 1.10, AWS credentials for the target account (`aws sts get-caller-identity` works).
- A Cloudflare API token with Zone Read, DNS Edit, Zone Settings Edit and SSL and
  Certificates (Origin CA) on the zone, exported in your shell (never in a file in the repo):
  `export CLOUDFLARE_API_TOKEN=...`
- The zone already on Cloudflare nameservers.
- An SSH key pair with the public key at `infra/.keys/finances-prod.pub`
  (`ssh-keygen -t ed25519 -f infra/.keys/finances-prod`). The private key never enters Terraform.

## Usage

```sh
# 1. State bucket (local state, once)
cd infra/bootstrap
terraform init
terraform apply                      # prints bucket_name

# 2. Main stack
cd ..
cp backend.hcl.example backend.hcl   # put the bucket name in it (gitignored)
cp terraform.tfvars.example terraform.tfvars   # real values (gitignored)
terraform init -backend-config=backend.hcl
terraform plan
terraform apply
```

After `apply`, wait for the first boot (about 5 minutes) and check it:

```sh
ssh -i infra/.keys/finances-prod ubuntu@<elastic_ip> 'cloud-init status --wait; ls /var/lib/finances-bootstrap.done; tail /var/log/finances-bootstrap.log'
```

The marker `/var/lib/finances-bootstrap.done` appears only when everything succeeded.
Continue with `deploy/README.md` section 4 (configuration) in `/home/ubuntu/finances-app`.

### Close SSH when done

```sh
terraform apply -var enable_ssh=false     # or -var admin_cidr=""
```

This removes the port 22 rule entirely. To reopen, apply again without the flag (the
current public IP is detected on every run) or set `admin_cidr` by hand. If your IP
changes, SSH stays blocked until the next `apply`.

### Destroy

`terraform destroy` removes everything except what has `prevent_destroy`: the data volume
(so the destroy **fails** while it is protected). To delete the data for real, remove the
`lifecycle` block in `compute.tf` on purpose, then destroy. The state bucket in
`infra/bootstrap` is protected the same way and has `force_destroy = false`. Take a
snapshot before destroying anything.

## Variables

| Name | Default | Meaning |
| --- | --- | --- |
| `zone_name` | required | Cloudflare zone |
| `hostname` | required | public host name inside the zone |
| `budget_email` | required | budget alert recipient |
| `region` | `us-east-2` | AWS region |
| `budget_limit_usd` | `30` | monthly cost budget |
| `instance_type` | `t4g.small` | arm64 instance type |
| `root_volume_size_gb` | `12` | root disk |
| `data_volume_size_gb` | `50` | data disk (can only grow; see "Growing the data disk") |
| `ssh_public_key_path` | `./.keys/finances-prod.pub` | public key registered on the VM |
| `enable_ssh` | `true` | `false` removes the SSH rule |
| `admin_cidr` | `null` (auto-detect /32) | SSH source; `""` closes SSH |
| `repo_url` | this repository (HTTPS) | cloned on the VM |
| `repo_ref` | `main` | branch, tag or commit checked out |
| `snapshot_time_utc` | `06:00` | DLM snapshot time |
| `snapshot_retention` | `7` | snapshots kept |

## Security notes

- The repository is public: `*.tfvars`, `*.tfstate*`, `.terraform/`, `backend.hcl`, `infra/.keys/`
  and key patterns are gitignored. Only `*.example` files with placeholders are committed.
  Check with `git check-ignore -v` before committing; never `git add -f` them.
- The Terraform **state contains the origin certificate private key** (and SSM values). It
  lives in the private, versioned, encrypted S3 bucket with TLS-only access; restrict who
  can read that bucket.
- The Cloudflare token is read from `CLOUDFLARE_API_TOKEN` only.
- `infra/.keys/` holds the SSH private key inside the working tree: it is ignored, but a
  forced add would leak it. Keeping it in `~/.ssh` is safer (change `ssh_public_key_path`).
- IMDSv2 is required with hop limit 1, so containers cannot reach the instance credentials.
- Backups: snapshots are a minimal safety net, not a backup strategy.

## Cost (approximate, verify in the AWS calculator)

About 22 USD/month: t4g.small ~12, root ~1, data 50 GB ~4, public IPv4 ~3.6, snapshots ~1-2 (approximate; check the AWS calculator).

## Growing the data disk

The volume is online-resizable (it can only grow, never shrink):

1. Raise `data_volume_size_gb` in `terraform.tfvars` and run `terraform apply` (an in-place change).
2. On the VM, with everything still running: `sudo resize2fs <device>` (the ext4 filesystem sits on the raw device, no partition table; find the device with `lsblk`, it is the one mounted at `/srv/finances`).
3. AWS requires a wait of several hours before the next modification of the same volume.
