"""A small HTTP service that registers accounts."""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

from accounts import normalize_email, validate_name

ACCOUNTS = {}

PAGE = b"""<!doctype html>
<form id="signup"><input name="email"><input name="name"><button>Sign up</button></form>
<p id="result"></p>
<script>
document.getElementById("signup").onsubmit = async (e) => {
  e.preventDefault();
  const body = JSON.stringify(Object.fromEntries(new FormData(e.target)));
  const r = await fetch("/accounts", {method: "POST", body});
  document.getElementById("result").textContent = (await r.json()).message;
};
</script>
"""


def register(email, name):
    email, name = normalize_email(email), validate_name(name)
    if email in ACCOUNTS:
        raise ValueError(f"{email} is taken")
    ACCOUNTS[email] = name
    return email


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.end_headers()
        self.wfile.write(PAGE)

    def do_POST(self):
        data = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        try:
            email = register(data.get("email", ""), data.get("name", ""))
            status, message = 201, f"registered {email}"
        except ValueError as e:
            status, message = 400, str(e)
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"message": message}).encode())


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 8000), Handler).serve_forever()
