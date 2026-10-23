# Working in `cli/`

## Factory

`cmdutil.Factory` (`cmdutil/factory.go`) is the single source of truth for shared deps: `IOStreams`, `Config`, `Prompter`, `HTTPClient`, `AgentManager`.

- Commands take deps off `*cmdutil.Factory` in `NewXxxCmd(f)` and inject into `XxxOptions`
- Do NOT call `iostreams.System()`, `prompter.New(...)`, `config.Load(...)`, or build clients directly — bypasses test seams and token refresh
- If the factory doesn't expose what you need, add it there — don't reach around it
- Leaf packages (`auth`, `clients`, etc.) must NOT import `cmdutil` — they take concrete deps as parameters

## Output

Text output lives in the leaf command file (`cmd/<group>/<verb>.go`), written through `iostreams` — never `io` or `fmt` to stdout/stderr directly.

- Errors: always `return render.Error(opts.IO, scope, err)` — never naked `return err`
- JSON mode: `render.JSONSuccess` / `render.JSONError` — the envelope structure is a wire contract
- New error codes go in `clierr` as stable constants (wire contract — don't rename or remove)

## Scope resolution

Leaf commands call `opts.ResolveScope(cmd, needsOrg, needsProject)` for however much scope they need — some commands only require org. Build a `render.Scope` with `opts.MakeScope(...)` from whatever was resolved.

## Command shape

`RunE` is the only place that touches `*cobra.Command`. Resolve flags and scope inside `RunE`, then call `runX(ctx, opts)` — never `runX(cmd, opts)`. Cobra types must not appear in `runX`, validators, or section builders.

This keeps `runX` directly unit-testable from a hand-built `*XxxOptions` without spinning up a root → group → leaf cobra tree (see `agent/get_test.go`, `agent/delete_test.go` for the canonical pattern).

## Wiring Options

Every Options field sourced from the Factory must be a direct reference — not a closure that forwards to one.

```go
// good — direct method value / field
Client:       f.AgentManager,
MakeScope:    f.Scope,
ResolveScope: f.ResolveOrgProject,

// bad — closure that just forwards
ResolveScope: func(cmd *cobra.Command) (string, string, error) {
    return f.ResolveOrgProject(cmd, true, true)
},
```

If the field type doesn't match the Factory method signature, change the field type (or add a helper on Factory). Don't paper over mismatches with closures.

**Exception:** Closures are fine for cobra completion functions (`RegisterFlagCompletionFunc`, `ValidArgsFunction`) where the API mandates a function value. See `cmdutil/org_override.go` and `cmdutil/project_override.go` for the canonical pattern.

## Testing

Unit tests build `*XxxOptions` directly — no cobra tree needed. See `agent/get_test.go` and `agent/delete_test.go` for the canonical pattern.

- Use `iostreams.Test()` for captured output
- Use a fake HTTP client or mock server for API calls
- Test `runX(ctx, opts)` — never the cobra `RunE` path

## Documentation

All exported functions and types must have doc comments. Internal (unexported) functions in shared packages (`cmdutil`, `render`, `clierr`, `clients`) should too. Leaf command internals (`cmd/<group>/*.go`) are lower priority — add doc comments when the intent isn't obvious from the name and signature.

Doc comments explain **why** something exists and when to use it — the code already explains the what. Don't restate the signature in prose.

## WSO2 Cloud

Cloud-only knowledge lives in `pkg/cloud` — keep it out of the instance-agnostic `login` / `auth` code.

- `amctl login --cloud` builds the instance URL from `cloud.APIURL` (gateway in `AMCTL_CLOUD_URL`, production by default, plus the API path) and sends the RFC 8707 resource indicator `cloud.Resource` (`urn:wso2:amp`). The cloud IdP is shared across WSO2 Cloud products and does not default to the Agent Manager resource server, so without it the token has no `amp:*` scopes
- The resource indicator is cloud-only and deliberately not a flag; re-login restores it from the stored instance (`auth.resource` in `~/.amctl/config`)
- Scopes always come from discovery (`/.well-known/oauth-protected-resource` → `scopes_supported`); don't add a client-side scope list

## Releases and updates

amctl has its own version line, independent of the platform.

- **Released by `.github/workflows/amctl_release.yaml`** (manual dispatch: source branch + semver). It tests, builds with `scripts/build-amctl.sh` (all platforms + `checksums.txt`), then tags `amctl/v<version>` and creates a **draft** release with `make_latest: false` so the platform release keeps the repo's "Latest" badge. A `-` suffix (`1.1.0-rc.1`) makes a pre-release. Publish the draft to ship it
- **The platform release does not ship amctl** — `release.yml` / `release-promote.yml` (`amp/v<version>`) have no amctl job. Don't add it back; two streams would publish conflicting `amctl` versions
- **Version stamping:** `pkg/version` (`Version`, `Commit`, `Date`) is set via `-ldflags` by `scripts/build-amctl.sh` (`make amctl-build` for the local platform, `make amctl-release-dry-run` for all). Plain `go build` reports `dev`
- **Update check (`pkg/update`):** reads only published, non-pre-release `amctl/v…` releases (following GitHub's `Link: rel="next"` pages), takes the highest version (skipping releases with no `amctl_v…` binaries), and links the `amctl_v<version>_<os>_<arch>` binary for the running platform, or the release page when that release has no binary for it. `cmd/login.go` (`warnIfOutdated`) runs it on cloud logins only: 3s timeout, never fails the login, skipped for `dev` builds
- **Keep the contract in sync:** the tag prefix (`amctl/v`) and asset names (`amctl_v<version>_<os>_<arch>.*`) are shared by the workflow, `scripts/build-amctl.sh` and `pkg/update` (where the repo and the `amctl` marker are defined once, as `repo` and `stream`) — change them together
- **Version ordering:** binaries attached to the earlier platform release `amp/v1.0.0` report `1.0.0`; keep `amctl/v` versions above that, or those users are never told to upgrade
- Users are pointed at the amctl releases listing (`update.ReleasesPageURL`, `releases?q=amctl`), never `/releases/latest` — that is the platform release

