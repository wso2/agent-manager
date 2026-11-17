# Agent Manager Service

## Overview

The Agent Manager Service is a core component of the Agent Management Platform that handles agent deployment, management, and governing AI agents.

## Folder Structure

```
agent-manager-service/
├── api/                        # HTTP API layer with HTTP handlers and routing
├── clients/                   # External service clients
├── cmd/                       # Binaries: amp (core and every module) and amp-core (core only)
├── config/                    # Configuration management
├── controllers/               # HTTP request controllers
├── db/                       # Database connection and utilities
├── db_migrations/            # Database schema migration files
├── db_types/                 # Custom database types
├── docs/                     # OpenAPI documentation
├── internal/                 # Command line (cli) and module host (modhost)
├── middleware/               # HTTP middleware (auth, logging, recovery)
├── modkit/                   # Contract between the core and a module
├── models/                   #  Data models and entities
├── modules/                  # Modules linked into the amp binary
├── repositories/             # Data access layer
├── scripts/                  # Development and Build scripts
│   ├── fmt.sh               # Code formatting
│   ├── gen_client.sh        # Client code generation
│   ├── lint.sh              # Code linting
│   ├── newline.sh           # Newline formatting
│   └── run_tests.sh         # Test execution
├── services/                 # Business logic layer
├── signals/                  # # Graceful shutdown handling
├── tests/                    # Test files
├── utils/                    # Utility functions
├── wiring/                   # Dependency injection
├── .air.toml                 # Air hot-reload configuration
├── .env                      # Environment variables (development)
├── Dockerfile                # Production container build
├── Dockerfile.dev            # Development container with hot-reload
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
└── Makefile                  # Build automation
```

## Prerequisites

- **Go**: Version 1.26.0 or later
- **PostgreSQL**: Version 12 or later
- **Make**: For build automation
- **air** go install github.com/air-verse/air@v1.67.1
- **moq**   go install github.com/matryer/moq@v0.5.3

## Local Development

### 1. Clone the Repository

```bash
git clone <repository-url>
cd agent-management-platform/agent-manager-service
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Set Up Database

### 4. Configurations
<!-- Update this section when adding new configs-->
The service uses environment variables for configuration. Create a `.env` file in the project root:


| **Key**                            | **Description**                                           |
|------------------------------------|-----------------------------------------------------------|
| `SERVER_HOST`                      | Host address where the server runs                        |
| `SERVER_PORT`                      | Port number for the server                                |
| `DB_HOST`                          | Database host address                                     |
| `DB_PORT`                          | Database port number                                      |
| `DB_USER`                          | Username for database authentication                      |
| `DB_PASSWORD`                      | Password for database authentication                      |
| `DB_NAME`                          | Name of the database                                      |
| `JWT_SIGNING_PRIVATE_KEY_PATH`     | Path to RSA private key for JWT signing                   |
| `JWT_SIGNING_PUBLIC_KEYS_CONFIG`   | Path to JSON config file containing public keys           |
| `JWT_SIGNING_ACTIVE_KEY_ID`        | Key ID for active signing key                             |
| `JWT_SIGNING_DEFAULT_EXPIRY`       | Default token expiry duration (e.g., "8760h" for 1 year)  |
| `JWT_SIGNING_ISSUER`               | Issuer claim for JWT tokens                               |
| `JWT_SIGNING_DEFAULT_ENVIRONMENT`  | Default environment for token claims                      |



### 5. Generate JWT Signing Keys

Generate RSA key pairs for JWT token signing:

```bash
cd agent-management-platform/agent-manager-service
make gen-keys
# or directly:
./scripts/gen_keys.sh
```

**Generated Artifacts (default key-1):**
- `keys/private.pem` - Private signing key
- `keys/public.pem` - Public verification key
- `keys/public-keys-config.json` - Public keys configuration with key ID "key-1"

**Key Rotation (generating additional keys):**
To generate keys with a different key ID for rotation:
```bash
./scripts/gen_keys.sh key-2
```

This produces:
- `keys/private-key-2.pem` - Private signing key for key-2
- `keys/public-key-2.pem` - Public verification key for key-2
- Update the `keys/public-keys-config.json` to include key-2

**Environment Variable Configuration:**
After generating keys, configure these environment variables in your `.env` file:

```bash
# Path to the private key file for signing tokens
JWT_SIGNING_PRIVATE_KEY_PATH=./keys/private.pem

# Active key ID (must match the key ID in public-keys-config.json)
JWT_SIGNING_ACTIVE_KEY_ID=key-1

# Path to the public keys configuration file
JWT_SIGNING_PUBLIC_KEYS_CONFIG=./keys/public-keys-config.json
```

**For key rotation:** When switching to a new key (e.g., key-2), update:
- `JWT_SIGNING_PRIVATE_KEY_PATH=./keys/private-key-2.pem`
- `JWT_SIGNING_ACTIVE_KEY_ID=key-2`

### 6. Run Database Migrations

```bash
cd agent-management-platform/agent-manager-service
ENV_FILE_PATH=.env go run ./cmd/amp migrate
```

The `amp` binary's subcommands:

| Command | What it does |
|---|---|
| `amp serve [--migrate] [--wait-for-migrations]` | Run the API server |
| `amp migrate` | Apply the core's migrations, then every compiled-in module's |
| `amp init thunder <dir> [gateway <dir>] [fileauth <dir>]` | Write component configuration and exit |
| `amp module list` | List compiled-in modules and their state |
| `amp module install <name> [--with-deps]`, `enable`, `disable`, `uninstall` | Change a module's state; running servers pick it up without a restart |

The flags of earlier releases (`-migrate`, `-server=false`, `-init-thunder=<dir>`, ...) still work.

Each module keeps its tables in `modules/<dir>/schema/NNN_description.yaml`, in the same format as `db_migrations/schema/lite.yaml`, with tables named `mod_<module>_<table>`. `make schema` renders them into `modules/<dir>/migrations/<database>/`, which the module embeds and returns from `Migrations()`.

### 7. Start Development Server

Using Make:

```bash
cd agent-management-platform/agent-manager-service
make run
```

or run Air directly:
```bash
cd agent-management-platform/agent-manager-service
air
```

The service will start on `http://localhost:8910` by default with hot-reloading enabled.

### 8. Run tests
```bash
cd agent-management-platform/agent-manager-service
make test
```

### 9. Development Tools

- **File Watcher**: `air` provides hot-reloading - watches for file changes and rebuilds/restarts automatically
- **Code Formatting**: `make fmt` to format code
- **Linting**: `make lint` to run linters
- **Testing**: `make test` to run tests
- **Generate wire dependencies**: `make wire`
- **Code Generation**: `make codegen` to generate wire dependencies and models
- **Model generation from the API specification** - `make spec`

### OpenAPI Specification

The API is documented using OpenAPI 3.0 specification in `docs/api_v1_openapi.yaml`.

### Agent Token Authentication

The service provides JWT-based authentication for external agents:

- **Token Generation**: `POST /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/token`
  - Generate a signed JWT token for an agent
  - Optional parameters: `environment` (query), `expires_in` (body)
  - Returns a Bearer token with configurable expiry

- **JWKS Endpoint**: `GET /auth/external/jwks.json`
  - Public endpoint for retrieving JSON Web Key Set
  - Used by clients to verify JWT signatures
  - No authentication required

Tokens include claims for:
- Component UID
- Environment UID
- Project UID
- Standard JWT claims (iss, sub, exp, iat, nbf)


