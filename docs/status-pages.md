# Status pages

Pulse serves one or more public status pages. Each page has its own domain, title, published flag, and selected monitor groups. Pulse resolves which page to show from the request Host header.

## The default page

Pulse has a default page. When a request arrives on a host that does not match any configured page, Pulse falls back to the default page, which shows all monitor groups.

## Creating a page

In the admin under Pages, create a page and set:

- **Custom domain.** The host that serves this page.
- **Title.** The page title.
- **Published flag.** Whether the page is visible.
- **Monitor groups.** The groups this page displays.

A page can also have its own logo and theme. See [Theming](theming.md) for per-page branding.

## Group selection and scoping

A page displays only the monitor groups you select for it. Incidents and maintenance are scoped to the monitors a page shows, so a page reflects only the services in its selected groups. The default page shows all groups.

## What the public page shows

A page renders:

- Overall status.
- Per-monitor uptime bars over a selectable range (90d, 60d, 30d, 15d, 1w, Today).
- Uptime percentage and average latency.
- The incident timeline and incident history.
- Maintenance sections (in-progress banner, upcoming, and history).
- An Atom feed, reachable from a Subscribe control.

All times render in the visitor's local timezone.

## Badges

Each page serves SVG badges you can embed in a README, a wiki or a dashboard. The badge reflects the page that matches the Host in its URL.

```markdown
![Status](https://status.example.com/badge/status.svg)
![API](https://status.example.com/badge/status.svg?monitor=API)
![Uptime](https://status.example.com/badge/uptime.svg?range=30d)
```

- `/badge/status.svg` shows the overall status of the page.
- `/badge/uptime.svg` shows uptime over `24h`, `7d`, `30d` or `90d`. The default is `30d`.
- Add `?monitor=<name or id>` for one monitor. It must be on the page, or the badge returns 404.

A badge never shows a monitor that is not on its page, a target or a raw error. An unpublished page has no badges. Browsers and proxies may cache a badge for 60 seconds. See [Status badges](api.md#status-badges) for the full reference.

## Custom domains and TLS

Pulse routes requests to pages by the Host header, but it does not terminate TLS itself. To serve a page such as `status.shreeda.xyz` over HTTPS, run a reverse proxy in front of Pulse that holds the certificates and forwards to Pulse on port 8080. The proxy passes the original Host header so Pulse can resolve the correct page.

Point a DNS record for each page domain at your reverse proxy. Repeat the proxy configuration for each additional page domain.

### Caddy example

Caddy obtains and renews certificates automatically.

```caddyfile
status.shreeda.xyz {
    reverse_proxy localhost:8080
}
```

To serve more page domains, add a block per domain:

```caddyfile
status.shreeda.xyz, status.example.com {
    reverse_proxy localhost:8080
}
```

### nginx example

With nginx, obtain certificates separately (for example with Certbot) and reference them in the server block.

```nginx
server {
    listen 443 ssl;
    server_name status.shreeda.xyz;

    ssl_certificate     /etc/letsencrypt/live/status.shreeda.xyz/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/status.shreeda.xyz/privkey.pem;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host $host;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

The `proxy_set_header Host $host` line preserves the original host so Pulse resolves the right page.
