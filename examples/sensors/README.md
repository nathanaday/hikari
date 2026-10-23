# The sensors example

A project repository shaped like an IoT monitoring product, small enough to build in seconds. Make it with `make example-sensors`, then run `make up COMPOSE=.hikari/demo/sensors/compose.yaml`.

| System | What it releases | How it runs in an environment |
|---|---|---|
| `edge` | a Go binary in a tarball (`linux-tarball`) | mounted into a Debian image; five run at once |
| `cloud` | an HTTP service (`http-service`), as an image that hikari builds | its own image |
| `console` | a web page and its nginx configuration (`static-site`) | mounted into nginx |
| `thresholds` | the threshold of each sensor (`data-files`) | mounted into the cloud |

The edge replays readings from files and posts each one to the cloud. The cloud opens an incident when a reading is over its sensor's threshold, and mails the operator. The console shows the incidents. In the test system, Mailpit stands in for the mail service.

The tester `sensors-e2e` proves the stories of `#main`. The hardware watchdog of a real edge device is out of reach of the test system.
