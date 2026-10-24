import re

_EMAIL = re.compile(r"^[^@\s]+@[^@\s]+\.[a-z]{2,}$", re.IGNORECASE)


def normalize_email(address):
    """Return the address trimmed, or raise ValueError."""
    address = address.strip()
    if not _EMAIL.match(address):
        raise ValueError(f"not an email address: {address!r}")
    return address


def validate_name(name):
    """Return the display name trimmed, or raise ValueError."""
    name = " ".join(name.split())
    if not 1 <= len(name) <= 64:
        raise ValueError("a name has 1 to 64 characters")
    return name
