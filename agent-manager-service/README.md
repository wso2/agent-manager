# Agent Manager Service

## Overview

The Agent Manager Service is a core component of the Agent Management Platform that handles agent deployment, management, and governing.=

## Folder Structure

```
agent-manager-service/
├── api/                        # HTTP API layer with HTTP handlers and routing
├── clients/                   # External service clients
├── config/                    # Configuration management
├── controllers/               # HTTP request controllers
├── db/                       # Database connection and utilities
├── db_migrations/            # Database schema migration files
├── db_types/                 # Custom database types
├── docs/                     # OpenAPI documentation
├── middleware/               # HTTP middleware (auth, logging, recovery)
├── models/                   #  Data models and entities
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
├── main.go                   # Application entry point
└── Makefile                  # Build automation
```

## Prerequisites

- **Go**: Version 1.25.0 or later
- **PostgreSQL**: Version 12 or later
- **Make**: For build automation
- **air** go install github.com/air-verse/air@latest
- **moq**   go install github.com/matryer/moq@latest

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


| **Key**        | **Description**                         |
|----------------|-----------------------------------------|
| `SERVER_HOST`  | Host address where the server runs       |
| `SERVER_PORT`  | Port number for the server               |
| `DB_HOST`      | Database host address                    |
| `DB_PORT`      | Database port number                     |
| `DB_USER`      | Username for database authentication     |
| `DB_PASSWORD`  | Password for database authentication     |
| `DB_NAME`      | Name of the database                     |



### 5. Run Database Migrations

```bash
cd agent-management-platform/agent-manager-service
ENV_FILE_PATH=.env go run . -migrate
```

### 6. Start Development Server

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

### 7. Run tests
```bash
cd agent-management-platform/agent-manager-service
make test
```

### 8. Development Tools

- **File Watcher**: `air` provides hot-reloading - watches for file changes and rebuilds/restarts automatically
- **Code Formatting**: `make fmt` to format code
- **Linting**: `make lint` to run linters
- **Testing**: `make test` to run tests
- **Generate wire dependencies**: `make wire`
- **Code Generation**: `make codegen` to generate wire dependencies and models
- **Model generation from the API specification** - `make spec`

## Scripts
Run make help to see all available commands.

## API Documentation

### OpenAPI Specification

The API is documented using OpenAPI 3.0 specification in `docs/api_v1_openapi.yaml`.


