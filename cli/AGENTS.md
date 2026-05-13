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
