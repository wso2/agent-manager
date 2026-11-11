# Upgrading workload secrets from OpenBao

Existing OpenChoreo `SecretReference` objects point to values in OpenBao. Changing the data plane's store does not copy those values. Keep the data plane on OpenBao during the transition and use the in-house store explicitly for new releases.

## Prepare the upgrade

1. Record the current data plane store name and back up the OpenBao data, Agent Manager database, and OpenChoreo resources. Inventory sensitive environment variables and file mounts per agent and environment, including credentials managed by the platform. Confirm you can retrieve or reissue each value before proceeding. Agent Manager's configuration API omits sensitive values from its responses.
2. Upgrade the Agent Manager chart. Wait for the in-house store with `kubectl wait --for=condition=Ready clustersecretstore/amp-secret-store --timeout=120s`. Preserve the existing data plane's `spec.secretStoreRef.name` and its OpenBao configuration. Use Helm directly for this upgrade. The quick-start and local setup scripts switch the planes to the in-house store.
3. Upgrade the platform resources extension with the following values. If the Agent Manager chart sets a custom `agentManagerService.config.secretStore.clusterSecretStoreName`, use that name in the readiness check and in both values below.

   ```yaml
   externalSecrets:
     clusterSecretStoreName: amp-secret-store
     workloadClusterSecretStoreName: amp-secret-store
   ```

   The workload setting applies to releases generated with the upgraded `agent-api` component type. Previously generated releases and their OpenBao references continue using the data plane's existing store. Preserve that store for every environment that still has a legacy workload. Perform the configuration changes below as part of each workload's migration.

## Import Agent Manager-managed secrets

Once the Agent Manager database migration has created the `secrets` table, use the upgraded Agent Manager binary to copy legacy values through the OpenChoreo secrets API. Keep the old store and its target-plane Secrets accessible until the import is complete.

Create `openbao-import.json` with one entry per legacy SecretReference. Get the secret name from the agent's old workload or release binding and verify its owner before adding it. The legacy namespace is shared, so use the owning organization's immutable OU ID. The manifest contains identifiers only.

```json
{
  "secrets": [
    {
      "organizationId": "01900000-0000-7000-8000-000000000001",
      "secretName": "my-agent-default-secrets"
    }
  ]
}
```

Pause writes to the secrets being copied and run the import from an upgraded Agent Manager pod. Substitute the actual namespace and pod name.

```bash
kubectl exec -i -n AMP_NAMESPACE AMP_POD -- \
  /go/bin/agent-manager-service -import-openbao-secrets < openbao-import.json
```

The command uses the pod's existing OpenChoreo credentials, database connection, and encryption key. It imports only Agent Manager-owned secrets available through the legacy API and leaves OpenBao and release bindings intact. A retry accepts identical rows and reports conflicting rows without overwriting them. The command exits unsuccessfully if any entries fail and preserves successful copies. Resolve failures before migrating the corresponding releases. Git credentials and externally managed secrets require their own credential update procedure.

## Move workloads

1. Start with a non-production environment. Create a new release using the upgraded component type through the normal deploy or promotion flow. Use the intended image and include the complete user-managed environment variables and file mounts, including unchanged settings. Keep the saved `secretRef` for imported entries. For entries you are re-saving manually, supply the actual value with `isSensitive: true`. Reissue any affected system-managed credentials whose values could not be imported through their owning features.
2. For an environment already pinned to a release that uses the upgraded component type, you can re-save the values through `PUT /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations`. Include `environmentName`, the complete user-managed `env` array, and the complete `files` array in the same request. Preserve each file's `key` and `mountPath`. The configuration endpoint updates the existing release binding, so saving configuration alone does not upgrade a release pinned to the old component type.
3. Confirm the new ExternalSecrets are Ready in the workload namespace and point to the in-house store. Check that the agent is serving and exercise the behavior that uses the migrated variables and files. Repeat for each environment.

If a rollout fails, keep OpenBao available. Restore the affected environment's previous release pin and binding configuration from the backup, then verify that it resolves its OpenBao secrets and serves again. Keep the OpenBao data and store in place until all affected releases, including any releases needed for rollback, have been accounted for.
