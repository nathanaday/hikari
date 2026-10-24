# server

A small HTTP service that registers accounts.

    python3 server.py              # serves http://localhost:8000
    python3 -m pytest tests        # unit tests and the browser test
    pip install playwright && playwright install chromium   # for tests/browser
