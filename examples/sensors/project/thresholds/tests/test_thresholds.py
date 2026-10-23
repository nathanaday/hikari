import json
from pathlib import Path

THRESHOLDS = json.loads((Path(__file__).parent.parent / "thresholds.json").read_text())


def test_has_default():
    assert "default" in THRESHOLDS


def test_every_threshold_is_a_positive_number():
    assert all(isinstance(v, (int, float)) and v > 0 for v in THRESHOLDS.values())
