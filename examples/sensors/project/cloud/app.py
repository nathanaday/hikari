"""The cloud: stores readings, opens an incident when a reading is over its
sensor's threshold, mails the operator, and lists the incidents."""

import json
import os
import smtplib
import sqlite3
import threading
import time
from email.message import EmailMessage
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Cloud:
    def __init__(self, db_path, thresholds, smtp_host="", smtp_port=1025, operator="operator@example.com"):
        self.db = sqlite3.connect(db_path, check_same_thread=False)
        self.lock = threading.Lock()
        self.thresholds = thresholds
        self.smtp_host, self.smtp_port, self.operator = smtp_host, smtp_port, operator
        with self.lock:
            self.db.execute("CREATE TABLE IF NOT EXISTS readings (sensor TEXT, value REAL, at REAL)")
            self.db.execute("CREATE TABLE IF NOT EXISTS incidents (id INTEGER PRIMARY KEY, sensor TEXT, value REAL, threshold REAL, at REAL)")

    def threshold(self, sensor):
        return self.thresholds.get(sensor, self.thresholds.get("default"))

    def add_reading(self, sensor, value):
        """Store a reading; return the incident it opened, or None."""
        if not isinstance(sensor, str) or not sensor or not isinstance(value, (int, float)):
            raise ValueError("a reading needs a sensor name and a number")
        now = time.time()
        limit = self.threshold(sensor)
        with self.lock:
            self.db.execute("INSERT INTO readings VALUES (?, ?, ?)", (sensor, value, now))
            if limit is None or value <= limit:
                self.db.commit()
                return None
            cur = self.db.execute("INSERT INTO incidents (sensor, value, threshold, at) VALUES (?, ?, ?, ?)", (sensor, value, limit, now))
            self.db.commit()
            incident = {"id": cur.lastrowid, "sensor": sensor, "value": value, "threshold": limit, "at": now}
        self.notify(incident)
        return incident

    def incidents(self):
        with self.lock:
            rows = self.db.execute("SELECT id, sensor, value, threshold, at FROM incidents ORDER BY id").fetchall()
        return [{"id": r[0], "sensor": r[1], "value": r[2], "threshold": r[3], "at": r[4]} for r in rows]

    def notify(self, incident):
        if not self.smtp_host:
            return
        msg = EmailMessage()
        msg["From"], msg["To"] = "cloud@example.com", self.operator
        msg["Subject"] = f"Incident {incident['id']}: {incident['sensor']} at {incident['value']}"
        msg.set_content(f"Sensor {incident['sensor']} read {incident['value']}, over its threshold of {incident['threshold']}.")
        with smtplib.SMTP(self.smtp_host, self.smtp_port, timeout=5) as s:
            s.send_message(msg)


def handler(cloud):
    class Handler(BaseHTTPRequestHandler):
        def reply(self, code, body):
            data = json.dumps(body).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path == "/health":
                self.reply(200, {"status": "ok"})
            elif self.path == "/incidents":
                self.reply(200, cloud.incidents())
            else:
                self.reply(404, {"error": "not found"})

        def do_POST(self):
            if self.path != "/readings":
                self.reply(404, {"error": "not found"})
                return
            try:
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
                incident = cloud.add_reading(body.get("sensor"), body.get("value"))
            except (ValueError, json.JSONDecodeError) as e:
                self.reply(400, {"error": str(e)})
                return
            self.reply(201, {"incident": incident})

        def log_message(self, *args):
            pass

    return Handler


def main():
    with open(os.environ.get("THRESHOLDS", "/opt/thresholds/thresholds.json")) as f:
        thresholds = json.load(f)
    cloud = Cloud(os.environ.get("DB", "/tmp/cloud.db"), thresholds, os.environ.get("SMTP_HOST", ""), int(os.environ.get("SMTP_PORT", "1025")))
    print("cloud listening on :8000", flush=True)
    ThreadingHTTPServer(("", 8000), handler(cloud)).serve_forever()


if __name__ == "__main__":
    main()
