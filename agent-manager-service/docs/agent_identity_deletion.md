# CP-managed AgentID deletion

Agent deletion removes the control-plane AgentID at the current project pipeline source, then carries its removal through each pipeline edge in order. The provider receives `SourceEnvironment` for each downstream target. A failed predecessor blocks its descendants; independent branches can continue. The agent and identity bindings remain available when remote cleanup fails, so retrying deletion is safe.

Environments with a recorded identity that are no longer in the current pipeline never block deletion. The pipeline is cleaned up and the agent is deleted first. Then the response reports the leftover environments, so the user can remove the identity there through the identity provider console. The optional `ProjectAgentIdentityPresenceProvider` narrows the list: an environment is left out only when the provider confirms the AgentID is gone there. A provider without it, or a check that fails, counts as "may still exist" and the environment is reported, so a provider that cannot inspect environments never leaves an agent undeletable.

When there are leftovers the API answers `200` with `{"warnings": [{"code": "AGENT_IDENTITY_OUTSIDE_PIPELINE", "message": "...", "environments": [...]}]}` instead of `204`. The console shows each message in full, and `amctl agent delete` prints it after the success line. The warning cannot be retried from the agent, which no longer exists, so removing the identity is a manual step in the identity provider.

The current pipeline source must still match the stored identity source. Moving the source requires reconciliation before deletion. Pipeline environments without a recorded target binding are included using the source AgentID, so an interrupted provisioning response cannot hide a deployed identity.

This behavior is limited to `CP_MANAGED`. Legacy deletion is unchanged. No database migration or background deletion job is introduced.

## Validation

Regression tests cover ordered deletion, source and intermediate failures, independent branches, missing target bindings, changed pipelines, leftover environments reported after the pipeline is cleaned up (confirmed present, confirmed gone, check unanswered), preserved credentials and records on failure, concurrency protection, the 200 response with warnings, and its untruncated console and CLI display. Provider implementations must additionally verify removal on the actual environment, including queued or partially refused operations.
