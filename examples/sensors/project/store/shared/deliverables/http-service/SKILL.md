---
name: http-service
description: How to make a release of the http-service deliverable - a container image that hikari builds from your Dockerfile, and a README of the API. Load it before you publish a release of http-service.
---

# An HTTP service

hikari builds the image when you publish, from the `Dockerfile` at the root of your scope, at the commit your map describes. So:

1. Make sure the `Dockerfile` builds and starts the service, then commit and call `mark_map_current`.
2. Write `README.md` in `/hikari/public/export/staging/http-service/`: the port, each endpoint with its request and answer, each environment variable with its default, the files the service reads, and a health check that a compose file can use.
3. Call `check_release`, then `publish_release`. The release names the image; other systems run it by that name.
