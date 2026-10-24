import pytest

import server


def test_register_twice():
    server.ACCOUNTS.clear()
    assert server.register("Ada@example.com", "Ada") == "ada@example.com"
    with pytest.raises(ValueError):
        server.register("ada@EXAMPLE.com", "Ada")
