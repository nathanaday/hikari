"""Registers an account from the terminal: register.py <email> <name>."""
import json
import sys
import urllib.request

from accounts import normalize_email, validate_name


def request(email, name):
    return {"email": normalize_email(email), "name": validate_name(name)}


def main(argv, url="http://localhost:8000/accounts"):
    if len(argv) != 3:
        print("usage: register.py <email> <name>", file=sys.stderr)
        return 2
    try:
        body = json.dumps(request(argv[1], argv[2])).encode()
    except ValueError as e:
        print(e, file=sys.stderr)
        return 1
    with urllib.request.urlopen(urllib.request.Request(url, data=body, method="POST")) as r:
        print(json.loads(r.read())["message"])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
