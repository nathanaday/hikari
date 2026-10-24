import pytest

from accounts import normalize_email, validate_name


def test_email_is_lowercased():
    assert normalize_email("  Ada@Example.COM ") == "ada@example.com"


def test_bad_email():
    with pytest.raises(ValueError):
        normalize_email("ada.example.com")


def test_name_spaces():
    assert validate_name("  Ada   Lovelace ") == "Ada Lovelace"


def test_long_name():
    with pytest.raises(ValueError):
        validate_name("x" * 65)
