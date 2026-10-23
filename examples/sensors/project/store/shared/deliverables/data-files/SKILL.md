---
name: data-files
description: How to make a release of the data-files deliverable - JSON files and a README of their format. Load it before you publish a release of data-files.
---

# Data files

1. Copy the JSON files into `/hikari/public/export/staging/data-files/`.
2. Write `README.md` there: the format of each file, the meaning and unit of each value, and its defaults.
3. Call `check_release`, then `publish_release`.
