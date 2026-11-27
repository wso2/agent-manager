# Running workflows manually

Manual workflows read [workflow-settings.json](workflow-settings.json) from the commit selected in the **Run workflow** branch or tag picker.

1. To use the defaults, select a branch or tag that contains these workflows. For a custom run, create a branch, edit the section matching the workflow filename without its extension, then commit and push the settings.
2. Select that branch or tag in **Run workflow**, or run `gh workflow run <workflow-file> --ref <settings-ref>` in the target repository.
3. For release and promotion workflows, fill in the version fields marked `null` before dispatching. Keep per-run release settings on the dispatch branch so the default branch remains available for the next release.

The selected ref supplies the workflow and settings. When a section has a `source-ref`, `source_ref`, or `branch` field, that field still selects the source to build or publish. Promotion selects existing release artifacts through `source_version`. For example, set `release.source-ref` to a commit SHA and `release.target_version` to the version you intend to publish.

For a nightly dry run, leave `nightly.dry_run` set to `true`. To test one instrumentation matrix cell, set the matrix filters or set `heavy_only` and `heavy_cell_id`. Cassette refresh PRs target the repository’s default branch. Scheduled runs use their built-in defaults. Workflows calling `publish-core-ui-npm.yml` continue to supply its `workflow_call` inputs.
