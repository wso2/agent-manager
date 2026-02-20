# Proposal: Secret Management for AI Agent Management Platform

## Summary

This proposal introduces a secure secret management system for the AI Agent Management Platform (AMP), enabling users to store sensitive configuration values (API keys, credentials, tokens) separately from regular environment variables. Secrets are stored in OpenBao (a Vault fork) in the control plane and securely synced to data plane workloads using the External Secrets Operator (ESO).

## Motivation

### Problem Statement

Currently, all environment variables for agent components are stored as plain text in the database and passed directly to workload specifications. This poses several challenges:

1. **Security Risk**: Sensitive values like API keys, database passwords, and tokens are stored unencrypted
2. **Audit Compliance**: No separation between configuration and secrets makes compliance difficult
3. **Secret Rotation**: No built-in mechanism for rotating secrets without redeploying
4. **Access Control**: Cannot apply different access policies to secrets vs. regular config

### Goals

- Provide secure storage for sensitive environment variables
- Enable secret injection into workloads without exposing values in specs
- Support secret synchronization from control plane to data plane
- Maintain backward compatibility with existing environment variable handling

### Non-Goals

- External secret store integration (AWS Secrets Manager, HashiCorp Vault Cloud, etc.) - future work
- Secret versioning and rollback - future work
- Dynamic secret generation - future work

## Proposal

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Control Plane                                   │
│                                                                              │
│  ┌──────────────────┐    ┌─────────────────────┐    ┌───────────────────┐  │
│  │   AMP Console    │───▶│  Agent Manager API  │───▶│  secret-manager   │  │
│  │  (isSecret flag) │    │   (CRUD secrets)    │    │    (OpenBao)      │  │
│  └──────────────────┘    └─────────────────────┘    └───────────────────┘  │
│                                    │                          │             │
│                                    │                          │             │
│                                    ▼                          │             │
│                          ┌─────────────────────┐              │             │
│                          │    OpenChoreo CP    │              │             │
│                          │  (Component specs)  │              │             │
│                          └─────────────────────┘              │             │
│                                    │                          │             │
└────────────────────────────────────│──────────────────────────│─────────────┘
                                     │                          │
                    ┌────────────────┘                          │
                    │  ComponentWorkload CR                     │ ClusterSecretStore
                    │  with secretKeyRef                        │
                    ▼                                           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                               Data Plane                                     │
│                                                                              │
│  ┌──────────────────┐    ┌─────────────────────┐    ┌───────────────────┐  │
│  │  ExternalSecret  │───▶│ External Secrets Op │───▶│   K8s Secret      │  │
│  │       CR         │    │       (ESO)         │    │                   │  │
│  └──────────────────┘    └─────────────────────┘    └───────────────────┘  │
│                                                              │              │
│                                                              ▼              │
│                                                     ┌───────────────────┐  │
│                                                     │   Agent Workload  │  │
│                                                     │ (env from secret) │  │
│                                                     └───────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Components

#### 1. OpenBao (Control Plane)

OpenBao is deployed via the new `wso2-amp-secrets-extension` Helm chart:

- **KV Secrets Engine**: Stores secrets at path `secret/data/<org>/<project>/<component>/<env-var-name>`
- **Kubernetes Auth**: Authenticates services using Kubernetes ServiceAccounts
- **Policies**:
  - `amp-secret-writer-policy`: Full CRUD for control plane services
  - `amp-secret-reader-policy`: Read-only for data plane workloads

#### 2. Secret Management Service (Control Plane)

A new service or extension to agent-manager-service that:

- Provides CRUD APIs for secrets
- Authenticates to OpenBao using Kubernetes auth
- Stores secrets in OpenBao KV store
- Does NOT store secret values in the database (only metadata)

#### 3. External Secrets Operator (Data Plane)

ESO is already deployed in the cluster. We add:

- **ClusterSecretStore**: `amp-openbao-store` - points to control plane OpenBao
- **ExternalSecret CRs**: Created per component to sync required secrets

#### 4. Console UI Changes

- Add `isSecret` toggle for environment variables
- Secret values shown as masked (`••••••••`)
- Secret values not returned in API responses after creation

### Data Flow

#### Creating a Secret

```
1. User sets environment variable with isSecret=true in Console
2. Console calls Agent Manager API: POST /components/{id}/secrets
3. Agent Manager API calls OpenBao: PUT secret/data/{org}/{project}/{component}/{name}
4. Database stores: { name, isSecret: true } (no value)
5. On deploy, ComponentWorkload CR includes:
   env:
     - name: API_KEY
       valueFrom:
         secretKeyRef:
           name: {component}-secrets
           key: API_KEY
```

#### Syncing Secrets to Data Plane

```
1. Build workflow creates ExternalSecret CR in data plane namespace
2. ESO reads ExternalSecret, authenticates to OpenBao via ClusterSecretStore
3. ESO creates/updates K8s Secret with values from OpenBao
4. Workload pod mounts secret as environment variable
```

### API Changes

#### Environment Variable Schema

```typescript
interface EnvironmentVariable {
  name: string;
  value?: string;           // Only for non-secrets
  isSecret: boolean;        // NEW: marks as secret
  valueFrom?: {             // NEW: for secret references
    secretKeyRef?: {
      name: string;
      key: string;
    };
  };
}
```

#### New Endpoints

```
POST   /api/v1/components/{componentId}/secrets
       Body: { name: string, value: string }
       Response: { name: string, isSecret: true }

PUT    /api/v1/components/{componentId}/secrets/{name}
       Body: { value: string }
       Response: { name: string, isSecret: true }

DELETE /api/v1/components/{componentId}/secrets/{name}
       Response: 204 No Content

GET    /api/v1/components/{componentId}/secrets
       Response: [{ name: string, isSecret: true }]  // No values returned
```

### Helm Chart: wso2-amp-secrets-extension

New Helm chart that deploys:

```yaml
# Chart.yaml
dependencies:
  - name: openbao
    version: "0.4.0"
    repository: https://openbao.github.io/openbao-helm

# Resources created:
# - OpenBao StatefulSet (via subchart)
# - ServiceAccount for ESO integration
# - ClusterSecretStore for ESO
# - Post-install jobs for OpenBao configuration:
#   - Enable Kubernetes auth
#   - Create reader/writer policies
#   - Create reader/writer roles
```

### OpenChoreo Integration

#### ComponentWorkflow Schema Update

```yaml
# amp-docker ComponentWorkflow
spec:
  workloadSpec:
    schema:
      properties:
        environmentVariables:
          items:
            properties:
              name:
                type: string
              value:
                type: string
              isSecret:
                type: boolean
              valueFrom:
                type: object
                properties:
                  secretKeyRef:
                    type: object
                    properties:
                      name:
                        type: string
                      key:
                        type: string
```

#### Workload Generation (Argo Workflow)

The `generate-workload-cr` step handles both plain values and secret references:

```yaml
{{- range .environmentVariables }}
- name: {{ .name }}
  {{- if .valueFrom }}
  valueFrom:
    secretKeyRef:
      name: {{ .valueFrom.secretKeyRef.name }}
      key: {{ .valueFrom.secretKeyRef.key }}
  {{- else }}
  value: {{ .value | quote }}
  {{- end }}
{{- end }}
```

### Security Considerations

1. **Secret Storage**: Secrets stored only in OpenBao, never in database
2. **Transport**: All communication over TLS (cluster internal)
3. **Access Control**:
   - Control plane services use writer role (full CRUD)
   - Data plane uses reader role (read-only)
4. **Audit**: OpenBao provides audit logging for all secret access
5. **Namespace Isolation**: Data plane can only read secrets for its namespace pattern (`dp*`)

### Configuration

#### values.yaml (wso2-amp-secrets-extension)

```yaml
openbao:
  enabled: true
  server:
    dev:
      enabled: true        # Disable for production
      devRootToken: "root" # Change for production

secretStore:
  name: amp-openbao-store
  path: secret
  version: v2
  auth:
    kubernetes:
      writerRole: amp-secret-writer-role
      readerRole: amp-secret-reader-role
      serviceAccountName: external-secrets-openbao

dataPlaneNamespacePattern: "dp*"
```

### Migration Path

1. **Phase 1**: Deploy OpenBao and ClusterSecretStore (this proposal)
2. **Phase 2**: Add secret CRUD APIs to agent-manager-service
3. **Phase 3**: Update Console UI with isSecret toggle
4. **Phase 4**: Update build workflows to create ExternalSecret CRs
5. **Phase 5**: Documentation and user guide

### Alternatives Considered

#### 1. Kubernetes Secrets Only

Store secrets directly in K8s Secrets without OpenBao.

**Pros**: Simpler, no additional components
**Cons**: No centralized management, secrets scattered across namespaces, harder to audit

#### 2. External Cloud Secret Managers

Integrate with AWS Secrets Manager, GCP Secret Manager, Azure Key Vault.

**Pros**: Enterprise-grade, managed service
**Cons**: Cloud-specific, additional cost, more complex setup

#### 3. Sealed Secrets

Use Bitnami Sealed Secrets for GitOps-friendly secret management.

**Pros**: GitOps compatible, encrypted at rest
**Cons**: Requires re-encryption on key rotation, no dynamic secrets

### Open Questions

1. Should we support secret references across components (shared secrets)?
2. How should secret deletion be handled when a component is deleted?
3. Should we implement secret rotation notifications?
4. Production deployment: How should OpenBao be deployed (HA, persistent storage)?

## Implementation Plan

### Phase 1: Infrastructure (Current)

- [x] Create `wso2-amp-secrets-extension` Helm chart
- [x] Deploy OpenBao with Kubernetes auth
- [x] Create ClusterSecretStore for ESO
- [x] Update install scripts

### Phase 2: API Layer

- [ ] Add secret CRUD endpoints to agent-manager-service
- [ ] Implement OpenBao client in Go
- [ ] Update database schema for isSecret flag
- [ ] Update component deployment logic

### Phase 3: Build Pipeline

- [ ] Update ComponentWorkflow schema
- [ ] Update Argo workflow templates
- [ ] Add ExternalSecret CR generation
- [ ] Test end-to-end secret injection

### Phase 4: UI

- [ ] Add isSecret toggle in environment variable form
- [ ] Implement secret masking in UI
- [ ] Add secret management section in component settings

## References

- [OpenBao Documentation](https://openbao.org/docs/)
- [External Secrets Operator](https://external-secrets.io/)
- [OpenChoreo Component Workflows](https://openchoreo.dev/docs/)
- [Kubernetes Secrets Best Practices](https://kubernetes.io/docs/concepts/configuration/secret/)
