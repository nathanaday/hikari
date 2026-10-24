import pytest

from register import main, request


def test_request():
    assert request(" ada@example.com", "Ada ") == {"email": "ada@example.com", "name": "Ada"}


def test_bad_email(capsys):
    assert main(["register.py", "nope", "Ada"]) == 1
    assert "not an email address" in capsys.readouterr().err


def test_usage():
    assert main(["register.py"]) == 2
