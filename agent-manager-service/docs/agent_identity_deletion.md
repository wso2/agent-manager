# CP-managed AgentID deletion

Agent deletion removes the control-plane AgentID at the current project pipeline source, then carries its removal through each pipeline edge in order. The provider receives `SourceEnvironment` for each downstream target. A failed predecessor blocks its descendants; independent branches can continue. The agent and identity bindings remain available when remote cleanup fails, so retrying deletion is safe.

Before remote writes, recorded environments outside the current pipeline are checked through the optional `ProjectAgentIdentityPresenceProvider`. If an AgentID is still deployed there, deletion returns HTTP 409 (`AGENT_IDENTITY_OUTSIDE_PIPELINE`). The console lists those environments and asks the user to remove the identity through the identity provider console, then retry. Providers without presence inspection require manual reconciliation rather than silently forgetting those bindings. Inspection failures never count as absence.

The current pipeline source must still match the stored identity source. Moving the source requires reconciliation before deletion. Pipeline environments without a recorded target binding are included using the source AgentID, so an interrupted provisioning response cannot hide a deployed identity.

This behavior is limited to `CP_MANAGED`. Legacy deletion is unchanged. No database migration or background deletion job is introduced.

## Validation

Regression tests cover ordered deletion, source and intermediate failures, independent branches, missing target bindings, changed pipelines, manual removal followed by retry, preserved credentials and records on failure, concurrency protection, the HTTP warning, and its untruncated console display. Provider implementations must additionally verify removal on the actual environment, including queued or partially refused operations.
