# Database ER diagram

Core tables of AMP Lite, rendered from [`db_migrations/schema/lite.yaml`](../db_migrations/schema/lite.yaml) by `docs/amp-lite/src/er_gen.py` (run through `docs/amp-lite/src/render.sh`). "owns" means the child is deleted with its parent.

Module tables (`mod_<module>_<table>`, such as the AI Gateway's), column rules and decisions are in the [AMP Lite database schema](../../docs/amp-lite/database-schema.md).

```mermaid
erDiagram
    organizations {
        id id PK
        text handle
        text display_name
        text description "nullable"
        text thunder_ou_id "nullable"
        text created_by "nullable"
        time created_at
        time updated_at
    }
    projects {
        id id PK
        id org_id FK
        text name
        text display_name
        text description "nullable"
        text created_by "nullable"
        time created_at
        time updated_at
    }
    environments {
        id id PK
        id org_id FK
        text name
        text display_name
        bool is_production
        time created_at
        time updated_at
    }
    agents {
        id id PK
        id org_id FK
        id project_id FK
        text name
        text display_name
        text description "nullable"
        text provisioning_type
        text created_by "nullable"
        time created_at
        time updated_at
    }
    agent_labels {
        id agent_id PK,FK
        id org_id FK
        text label_key PK
        text label_value "nullable"
    }
    agent_identities {
        id id PK
        id agent_id FK
        id org_id FK
        id environment_id FK
        text thunder_agent_id "nullable"
        text thunder_client_id "nullable"
        text status
        int attempt_count
        longtext last_error "nullable"
        time next_retry_at "nullable"
        text claimed_by "nullable"
        time claimed_until "nullable"
        text requested_by "nullable"
        time created_at
        time updated_at
    }
    secrets {
        id id PK
        id org_id FK
        text owner_kind
        id owner_id
        text provider
        binary ciphertext "nullable"
        text key_id "nullable"
        text external_ref "nullable"
        int version
        time created_at
        time updated_at
    }
    event_destinations {
        id id PK
        id org_id FK
        text name
        text description "nullable"
        text kind
        json config
        id secret_id FK "nullable"
        text availability
        text created_by "nullable"
        time created_at
        time updated_at
    }
    event_destination_projects {
        id destination_id PK,FK
        id project_id PK,FK
        id org_id FK
    }
    event_subscriptions {
        id id PK
        id org_id FK
        text scope
        id project_id FK "nullable"
        id agent_id FK "nullable"
        text name
        text description "nullable"
        id destination_id FK
        bool enabled
        text created_by "nullable"
        time created_at
        time updated_at
    }
    event_subscription_types {
        id subscription_id PK,FK
        text event_type PK
        id org_id FK
    }
    event_subscription_environments {
        id subscription_id PK,FK
        id environment_id PK,FK
        id org_id FK
    }
    event_outbox {
        id id PK
        id org_id FK
        json body
        time available_at
        text claimed_by "nullable"
        time claimed_until "nullable"
    }
    event_deliveries {
        id id PK
        id org_id FK
        id destination_id FK
        text event_id
        text event_type
        text event_scope "nullable"
        id project_id "nullable"
        id agent_id "nullable"
        id environment_id "nullable"
        text status
        int attempts
        int response_code "nullable"
        longtext last_error "nullable"
        time delivered_at "nullable"
        text claimed_by "nullable"
        time claimed_until "nullable"
        time created_at
        time updated_at
    }
    event_delivery_subscriptions {
        id delivery_id PK,FK
        id subscription_id PK
    }
    audit_events {
        id id PK
        id org_id "nullable"
        time occurred_at
        int schema_version
        text action
        text action_class
        int severity
        text outcome
        text actor_type
        text actor_id "nullable"
        text actor_display "nullable"
        text actor_token_id "nullable"
        text on_behalf_of "nullable"
        text auth_method "nullable"
        text surface
        text source_ip "nullable"
        text user_agent "nullable"
        text correlation_id "nullable"
        text request_method "nullable"
        longtext request_path "nullable"
        text resource_type "nullable"
        text resource_id "nullable"
        text resource_name "nullable"
        text project_name "nullable"
        text environment_name "nullable"
        int status_code "nullable"
        text error_code "nullable"
        longtext error_message "nullable"
        text required_permission "nullable"
        int duration_ms "nullable"
        json details "nullable"
    }
    usage_events {
        id id PK
        id org_id "nullable"
        time occurred_at
        text source
        text feature_code
        text username "nullable"
        text session_id "nullable"
        int status_code "nullable"
        text product_version
        json dimensions "nullable"
    }
    locks {
        text name PK
        text owner "nullable"
        time expires_at "nullable"
    }
    modules {
        text name PK
        text version
        text state
        time created_at
        time updated_at
    }
    module_generation {
        int id PK
        bigint value
    }
    module_migrations {
        text module PK
        text version PK
        text checksum
        time applied_at
    }
    module_events {
        id id PK
        id org_id FK
        text type
        text source
        text subject "nullable"
        json data
        time created_at
    }
    module_event_deliveries {
        id event_id PK,FK
        text module PK
        int attempts
        time available_at
        text claimed_by "nullable"
        text last_error "nullable"
        time failed_at "nullable"
    }
    workload_credentials {
        id id PK
        text module
        text workload
        id org_id FK "nullable"
        text subject
        text secret_hash
        time created_at
        time revoked_at "nullable"
    }
    workload_instances {
        id credential_id PK,FK
        text instance PK
        text module
        text workload
        text protocol
        text version
        time first_seen
        time last_seen
    }
    organizations ||--o{ projects : has
    organizations ||--o{ environments : has
    projects ||--o{ agents : has
    agents ||--o{ agent_labels : owns
    agents ||--o{ agent_identities : owns
    environments ||--o{ agent_identities : has
    organizations ||--o{ secrets : has
    organizations ||--o{ event_destinations : has
    secrets ||--o{ event_destinations : has
    event_destinations ||--o{ event_destination_projects : owns
    projects ||--o{ event_destination_projects : has
    agents ||--o{ event_subscriptions : owns
    projects ||--o{ event_subscriptions : has
    event_destinations ||--o{ event_subscriptions : has
    event_subscriptions ||--o{ event_subscription_types : owns
    event_subscriptions ||--o{ event_subscription_environments : owns
    environments ||--o{ event_subscription_environments : has
    organizations ||--o{ event_outbox : has
    event_destinations ||--o{ event_deliveries : owns
    event_deliveries ||--o{ event_delivery_subscriptions : owns
    organizations ||--o{ module_events : owns
    module_events ||--o{ module_event_deliveries : owns
    organizations ||--o{ workload_credentials : owns
    workload_credentials ||--o{ workload_instances : owns
```
