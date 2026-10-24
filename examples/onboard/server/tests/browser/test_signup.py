"""Drives the sign-up page in a real browser. Needs Playwright and Chromium."""
import threading
from http.server import HTTPServer

from playwright.sync_api import sync_playwright

import server


def test_signup_page():
    httpd = HTTPServer(("127.0.0.1", 0), server.Handler)
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    with sync_playwright() as p:
        page = p.chromium.launch().new_page()
        page.goto(f"http://127.0.0.1:{httpd.server_port}/")
        page.fill("input[name=email]", "Grace@Example.com")
        page.fill("input[name=name]", "Grace")
        page.click("button")
        page.wait_for_selector("#result:has-text('registered')")
        assert page.text_content("#result") == "registered grace@example.com"
    httpd.shutdown()
