# E2E Test Guide

This directory contains black-box end-to-end tests for agent-manager. Tests use
[Ginkgo](https://onsi.github.io/ginkgo/) + [Gomega](https://onsi.github.io/gomega/)
and run against a real deployment (the local Compose stack, `make up`, by default).

## Two test stacks

The suite has two independent stacks. Choose the one that matches what you are
testing.

| Stack | Use for | Entry package |
|-------|---------|---------------|
| **HTTP** | Direct API coverage, complex request/response shapes, async polling | `operations/`, `tests/` |
| **CLI** | `amctl` black-box coverage — exit codes, JSON envelopes, UX | `framework/amctl/`, `operations/cli/`, `tests/cli/` |

Both stacks share `framework/` for configuration, readiness waits, naming
prefixes, and stale cleanup.

## Layout

```
test/e2e/
framework/          shared config, HTTP client, auth, naming, helpers
operations/         HTTP operation wrappers per resource
  project/
  agent/
  ...
operations/cli/     CLI operation wrappers per resource (mirror operations/)
  project/
  agent/
  gateway/
  llmprovider/
tests/              HTTP Ginkgo specs
  project/
  agent/
  ...
tests/cli/          CLI Ginkgo specs
  project/          CRUD lifecycle
  agent/            agent CRUD (keyless)
  llmprovider/      llm-provider CRUD (keyless)
testsetup/          suite-wide setup / teardown / stale cleanup
```

## Adding HTTP tests

1. Put reusable HTTP helpers in `operations/<resource>/`. Each function should:
   - Accept `g Gomega` and `*framework.AMPClient`.
   - Use `client.Post|Get|Put|Delete` and `framework.ExpectStatus`.
   - Decode with `framework.DecodeBody[T]`.
2. Add Ginkgo specs under `tests/<resource>/`.
3. In `suite_test.go` create a package-level `*framework.AMPClient` in
   `BeforeSuite` (see `tests/project/suite_test.go`).
4. Name test resources with the appropriate `framework.*Prefix` constant
   (`E2EProjectPrefix`, `E2EAgentPrefix`, etc.) so the stale sweep reaps them.

Example pattern:

```go
func CreateWidget(g Gomega, client *framework.AMPClient, org, name string) framework.WidgetResponse {
    resp, err := client.Post(fmt.Sprintf("/api/v1/orgs/%s/widgets", org), framework.CreateWidgetRequest{Name: name})
    g.Expect(err).NotTo(HaveOccurred())
    defer resp.Body.Close()
    framework.ExpectStatus(g, resp, 202)
    return framework.DecodeBody[framework.WidgetResponse](g, resp)
}
```

## Adding CLI tests

CLI tests shell out to the real `amctl` binary and parse its `--json` envelope.

### 1. Reuse the harness

Each CLI suite declares a package-level harness:

```go
var H = amctl.RegisterSuite()
```

`amctl.RegisterSuite()` builds (or locates) the binary once, creates an
isolated `$HOME` per parallel process, and runs `amctl login`.

### 2. Add resource operations

Create `operations/cli/<resource>/<resource>_operations.go` as thin wrappers
around the harness — one directory per resource (Go allows one package per
directory, so resources cannot share `operations/cli/`). The package
(`cli<resource>`) mirrors `operations/<resource>/` and only depends on
`framework/amctl`.

Each operation function:

- Accepts `g Gomega`, `*amctl.Harness`, and command args.
- Calls `h.Run("<group>", "<verb>", ...)`.
- Decodes success with `amctl.DecodeData[T](g, res)`.
- Returns errors with `res.ExpectError(g)` when testing failures.

Example:

```go
// operations/cli/widget/widget_operations.go
package cliwidget

func CreateWidget(g Gomega, h *amctl.Harness, org, name string) Widget {
    return amctl.DecodeData[Widget](g, h.Run("widget", "create", name, "--org", org, "--json"))
}
```

### 3. Add specs

Specs live under `tests/cli/<resource>/`. Follow the project CRUD example:

- Use `Label("cli", "<resource>")` and `Ordered` for lifecycle specs.
- Generate names with `framework.E2E<resource>Prefix` + a short UUID suffix.
- `DeferCleanup` a best-effort delete so a failing spec does not leak.
- Assert exit codes and envelope shapes, not text output.

Example:

```go
var _ = Describe("amctl widget CRUD", Label("cli", "widget"), Ordered, func() {
    name := framework.E2EWidgetPrefix + uuid.New().String()[:8]

    BeforeAll(func() {
        DeferCleanup(func() { _ = H.Run("widget", "delete", name, "--org", H.Org(), "--yes", "--json") })
    })

    It("creates a widget", func() { cliwidget.CreateWidget(Default, H, H.Org(), name) })
    It("gets the widget", func() { cliwidget.GetWidget(Default, H, H.Org(), name) })
})
```

## Core CLI mechanics

- **Binary**: set `AMCTL_BIN` to use a pre-built binary; otherwise the suite
  builds `./cmd/amctl` from the `cli/` module once per run.
- **Isolation**: each Ginkgo process gets its own temp `$HOME`, so
  `~/.amctl/config` cannot collide.
- **Auth**: real `amctl login --client-id <IDP_CLIENT_ID> --client-secret ...`
  using the same credentials as the HTTP stack. If a command 403s because OIDC
  discovery scopes are too narrow, surface it; do not work around it.
- **Envelope**: every command appends `--json`. Success writes
  `{instance,org,project,data}`; failure writes `{...,error:{status,code,message,...}}`.
  Use `amctl.DecodeData[T]` or `Result.ExpectError`; never parse free-form text.
- **Stdin**: commands run with `Stdin` nil so the CLI sees a non-terminal and
  never prompts.

## Naming and cleanup

- Use the framework constants (`E2EProjectPrefix`, `E2EAgentPrefix`, etc.) for
  every resource name.
- Delete resources in `DeferCleanup` or the final spec leg.
- `testsetup/cleanup.go` sweeps stale resources older than one hour as a
  backstop; the prefixes are what let it identify test-owned objects.

## Configuration

`framework.LoadConfig()` reads the usual environment variables:

- `AMP_API_BASE_URL` (default `http://localhost:9000`)
- `IDP_TOKEN_URL`, `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET`
- `DEFAULT_ORG`, `DEFAULT_PROJECT`, `DEFAULT_ENV`
- `AMCTL_BIN` — optional pre-built CLI binary for CLI specs

Set these in `test/e2e/.env` or export them before running tests.

## Running tests

```bash
# All e2e tests
ginkgo ./tests/...

# HTTP project tests only
ginkgo ./tests/project/...

# CLI project tests only
ginkgo ./tests/cli/project/...

# With a pre-built CLI binary
AMCTL_BIN=/path/to/amctl ginkgo ./tests/cli/...
```

`ginkgo ./tests/...` globs both HTTP and CLI packages, so new CLI specs are
picked up automatically with no CI change.

## Rules of thumb

- **HTTP operations** go in `operations/<resource>/`; **CLI operations** go in
  `operations/cli/<resource>/`. The `agent` suite covers agent CRUD. Every e2e
  run executes all suites together. The `agent mcp` specs additionally require a
  running AI gateway in `DefaultEnv` and reachability of
  `framework.TestMCPServerURL`, since an mcp config can only bind a
  gateway-deployed catalog proxy.
- Keep `framework/amctl` resource-agnostic. Never add project/agent/etc.
  knowledge to the harness.
- CLI specs assert exit codes and JSON envelopes, not human-readable text.
- Always clean up after tests; use framework prefixes so the stale sweep can
  recover from leaks.
