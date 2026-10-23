import json
import threading
import urllib.request
from http.server import ThreadingHTTPServer

import pytest

from app import Cloud, handler


@pytest.fixture
def cloud(tmp_path):
    return Cloud(str(tmp_path / "c.db"), {"default": 50, "oven": 200})


def test_reading_under_threshold_opens_nothing(cloud):
    assert cloud.add_reading("s1", 49) is None
    assert cloud.incidents() == []


def test_reading_over_threshold_opens_incident(cloud):
    incident = cloud.add_reading("s1", 51)
    assert incident["threshold"] == 50
    assert [i["sensor"] for i in cloud.incidents()] == ["s1"]


def test_sensor_threshold_wins_over_default(cloud):
    assert cloud.add_reading("oven", 150) is None


def test_bad_reading_refused(cloud):
    with pytest.raises(ValueError):
        cloud.add_reading("", 1)


def test_http_api(cloud):
    srv = ThreadingHTTPServer(("127.0.0.1", 0), handler(cloud))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{srv.server_address[1]}"
    req = urllib.request.Request(base + "/readings", data=json.dumps({"sensor": "s1", "value": 99}).encode(), method="POST")
    with urllib.request.urlopen(req) as r:
        assert r.status == 201
    with urllib.request.urlopen(base + "/incidents") as r:
        assert json.load(r)[0]["value"] == 99
    srv.shutdown()
