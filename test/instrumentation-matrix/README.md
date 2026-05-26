# AMP Instrumentation Matrix

Compatibility-matrix test suite. See
[`references/INSTRUMENTATION-MATRIX-DESIGN.md`](../../references/INSTRUMENTATION-MATRIX-DESIGN.md)
for the architecture. Full operational docs in
[`references/INSTRUMENTATION-MATRIX.md`](../../references/INSTRUMENTATION-MATRIX.md)
(see Phase 9).

## Quickstart

```bash
nox -s emission                                 # full matrix
nox -s emission -k langchain                    # filter
nox -s emission -- --cell-id=<id>               # single cell
```
