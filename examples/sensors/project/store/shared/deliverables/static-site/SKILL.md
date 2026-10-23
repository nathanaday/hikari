---
name: static-site
description: How to make a release of the static-site deliverable - the page, its scripts, and the nginx configuration that serves it. Load it before you publish a release of static-site.
---

# A static site

1. Copy the page, its scripts, its styles, and `nginx.conf` into `/hikari/public/export/staging/static-site/`.
2. Write `README.md` there: where nginx must mount the files (`/usr/share/nginx/html`, and `nginx.conf` at `/etc/nginx/conf.d/default.conf`), which services the configuration forwards to, and what the page shows.
3. Call `check_release`, then `publish_release`.
