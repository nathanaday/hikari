# The form of the synthesis

```markdown
# Synthesis

## Duplicated code

### units
- Copies: `edge/src/units.cpp`, `cloud/app/units.py`.
- Differences: the edge rounds a temperature to one decimal; the cloud keeps two.
- Proposal: one library, `units`, with a C++ and a Python binding. Reason (library): two consumers need it, and their copies already differ.

## Boundaries

For each system of the target: what it owns, and the reason for its boundary, of one of three kinds (library; client and server; scope in a repository). Then each split you considered and decided against, and why.

## Left for integration

- `cloud/tests/ui/test_dashboard.py`: needs a browser. Stays in cloud, outside every test command.
- The edge sends a reading that the cloud stores: no test covers it. A story of the public wiki.

## Questions

1. The two copies of the temperature conversion differ: the edge rounds to one decimal, and the cloud keeps two. Which behavior is right for the library?
2. ...
```

A question names what the user must choose and the options. Keep each to two or three sentences.
