from pathlib import Path

ROOT = Path(__file__).parent.parent


def test_page_has_incident_table():
    html = (ROOT / "index.html").read_text()
    assert 'id="incidents"' in html and 'src="console.js"' in html


def test_script_reads_incidents_from_the_api():
    assert "fetch('/api/incidents')" in (ROOT / "console.js").read_text()


def test_nginx_forwards_api_to_cloud():
    conf = (ROOT / "nginx.conf").read_text()
    assert "location /api/" in conf and "proxy_pass http://cloud:8000/" in conf
