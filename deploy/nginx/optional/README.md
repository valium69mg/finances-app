Optional nginx settings, included at `http` level (`/etc/nginx/optional/*.conf`).
Only files ending in `.conf` are loaded; the `.example` files are templates.

- `authenticated-origin-pulls.conf.example`: make nginx accept TLS connections only from
  Cloudflare (mutual TLS). See deploy/README.md.
