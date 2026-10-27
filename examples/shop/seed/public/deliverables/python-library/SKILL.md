---
name: python-library
description: Build the python-library product of your partition - the importable package and its user-facing docs - in /hikari/staging/python-library/. Load it when you are asked to publish a release of python-library.
---

# Make the python-library product

The product is your partition as an importable Python package, with docs for an engineer in another team who will add it to their `PYTHONPATH` and who cannot see your code. Build it in `/hikari/staging/python-library/`. Put nothing else there.

## The package

1. Copy the package directory of your partition, the one with `__init__.py`, to the top of the staging directory. Copy only the modules that a user imports: no tests, no `__pycache__`, no private documents.
2. Check that the package imports from the staging directory with no other path on `PYTHONPATH`, for example `cd /hikari/staging/python-library && python3 -c "import <package>"`.

## The docs

Write `README.md` at the top of the staging directory, and more pages under `docs/` when one page would be too long, linked from the README. Their reader cannot see the code, so name nothing inside your repo. Use the names of the vocabulary (`get_vocabulary`) for domain terms and interfaces. The docs must cover:

1. What the package is and what it is for, in two or three sentences.
2. How to use it: add the release's `product/` directory to `PYTHONPATH`, then the import.
3. Each interface that the release provides, from your public map: what it does, its inputs and outputs, its errors, and a short example with a one-line explanation.
4. Its limits and known problems, from the map's findings.
5. What changed in this version, compared with the previous release in `/hikari/self/releases/python-library/latest/`, when there is one.

Follow the policies in `/hikari/public/policies/`.

## Publish

Run `check_release` with `python-library` and fix every problem. Then run `publish_release` with `python-library`, the version, and notes that say what changed.
