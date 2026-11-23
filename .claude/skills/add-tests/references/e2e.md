# End-to-end tests (test/e2e)

Background: `test/e2e/README.md` (layout, parallel processes) and `test/e2e/AGENTS.md` (HTTP and CLI stacks).

## HTTP specs

Existing suites: `tests/agent` (external agent lifecycle), `tests/project`, `tests/llmprovider`, `tests/mcpproxy`, and `tests/cli/{agent,project,llmprovider}`. Extend the suite that owns the resource before adding a new one.

1. Raw API call in `operations/<domain>/` — one function per call, asserting the status there, no test logic.
2. Spec in `tests/<domain>/` — a Ginkgo package with a `suite_test.go`. A new domain needs a new suite.
3. Shared fixtures through `testsetup/` (`EnsureProject` and friends), which check before they create.
4. Hooks: `BeforeSuite` only builds `Client`/`Cfg`; a resource several `Describe` blocks share goes in `SynchronizedBeforeSuite`; one used by a single `Describe` goes in `BeforeAll`.
5. Name resources with a prefix constant from `framework/shared_agent.go` (`E2EProjectPrefix` is `e2e-test-`, CLI specs use `E2ECLIProjectPrefix`) plus a short UUID, and `DeferCleanup` a best-effort delete.
6. Label the `Describe` (`Label("<domain>")`), and use `Ordered` for lifecycle flows.
7. Needs a model provider key? `if Cfg.OpenAIAPIKey == "" { Skip("OPENAI_API_KEY is not set …") }` at the top of the spec.

## CLI specs (`amctl` black box)

- `var H = amctl.RegisterSuite()` in the suite; it builds `./cmd/amctl` unless `AMCTL_BIN` is set.
- Thin wrappers in `operations/cli/<resource>/` (`package cli<resource>`) calling `h.Run(...)` and decoding with `amctl.DecodeData[T]`.
- Specs in `tests/cli/<resource>/`, `Label("cli", "<resource>")`; assert exit codes and `--json` envelopes, not text.

## Run

```bash
make up                                     # at the repo root, once
cp test/e2e/.env.example test/e2e/.env      # once; OPENAI_API_KEY optional
make e2e-test FOCUS="<your spec>"           # one spec, one process
make e2e-test SUITE=<domain>                # one suite
cd test/e2e && go vet ./...                 # compiles every suite
```

CI runs every suite on each PR to `amp-2.x` (`.github/workflows/e2e.yaml`), except documentation-only ones.
