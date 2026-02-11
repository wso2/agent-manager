# API Platform Service Client

This package provides a Go client wrapper for the WSO2 API Platform service, following the same pattern as the `openchoreosvc` client.

## Architecture

The client is structured in two main parts:

1. **Generated Code** (`gen/` directory)
   - Auto-generated from OpenAPI specification using `oapi-codegen`
   - Contains low-level HTTP client and type definitions
   - Should not be modified manually

2. **Wrapper Client** (`client/` directory)
   - High-level, user-friendly interface
   - Handles type conversions between generated types and domain types
   - Provides consistent error handling
   - Includes retry logic and authentication

## Files

### Generated Code
- `gen/types.gen.go` - Generated type definitions from OpenAPI spec
- `gen/client.gen.go` - Generated HTTP client from OpenAPI spec
- `gen/oapi-codegen.yaml` - Configuration for type generation
- `gen/oapi-codegen-client.yaml` - Configuration for client generation

### Wrapper Client
- `client/client.go` - Main client implementation with `GatewayClient` interface
- `client/types.go` - Domain types for requests and responses
- `client/constants.go` - Constants (functionality types, status values, etc.)
- `client/utils.go` - Helper functions for type conversions
- `client/errors.go` - Error handling and HTTP status code mapping

## Usage

### Configuration

The client requires configuration via environment variables or direct `Config` struct with an `AuthProvider`:

```go
import (
    "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/auth"
    "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
)

// Create auth provider (handles token fetching and caching)
authProvider := auth.NewAuthProvider(auth.Config{
    TokenURL:     "https://idp.example.com/oauth2/token",
    ClientID:     "your-client-id",
    ClientSecret: "your-client-secret",
})

// Create client config with auth provider
cfg := &client.Config{
    BaseURL:      "https://api-platform.example.com",
    AuthProvider: authProvider,
    RetryConfig: requests.RequestRetryConfig{
        MaxRetries:   3,
        RetryWaitMin: 1 * time.Second,
        RetryWaitMax: 10 * time.Second,
    },
}

// Create gateway client
gatewayClient, err := client.NewGatewayClient(cfg)
if err != nil {
    log.Fatal(err)
}
```

### Environment Variables

```bash
# Required
API_PLATFORM_ENABLED=true
API_PLATFORM_BASE_URL=https://api-platform.example.com

# OAuth2 Authentication (recommended)
API_PLATFORM_AUTH_TYPE=oauth2
API_PLATFORM_CLIENT_ID=your-client-id
API_PLATFORM_CLIENT_SECRET=your-client-secret
API_PLATFORM_TOKEN_URL=https://idp.example.com/oauth2/token

# Optional
API_PLATFORM_PROJECT_NAME=gateway
API_PLATFORM_TIMEOUT_SECONDS=30
API_PLATFORM_CACHE_ENABLED=true
API_PLATFORM_CACHE_TTL_SECONDS=60
```

### Gateway Operations

#### Create Gateway

```go
ctx := context.Background()

req := client.CreateGatewayRequest{
    Name:              "my-gateway",
    DisplayName:       "My Gateway",
    Vhost:             "api.example.com",
    FunctionalityType: client.FunctionalityTypeRegular,
    Description:       ptrString("Production API Gateway"),
    IsCritical:        ptrBool(true),
}

gateway, err := gatewayClient.CreateGateway(ctx, req)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Created gateway: %s\n", gateway.ID)
```

#### Get Gateway

```go
gateway, err := gatewayClient.GetGateway(ctx, "gateway-uuid")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Gateway: %s (%s)\n", gateway.Name, gateway.DisplayName)
```

#### List Gateways

```go
gateways, err := gatewayClient.ListGateways(ctx)
if err != nil {
    log.Fatal(err)
}

for _, gw := range gateways {
    fmt.Printf("- %s: %s\n", gw.Name, gw.DisplayName)
}
```

#### Update Gateway

```go
req := client.UpdateGatewayRequest{
    DisplayName: ptrString("Updated Gateway Name"),
    Description: ptrString("Updated description"),
    IsCritical:  ptrBool(false),
}

gateway, err := gatewayClient.UpdateGateway(ctx, "gateway-uuid", req)
if err != nil {
    log.Fatal(err)
}
```

#### Delete Gateway

```go
err := gatewayClient.DeleteGateway(ctx, "gateway-uuid")
if err != nil {
    log.Fatal(err)
}
```

## Functionality Types

The client supports three types of gateway functionality:

- `FunctionalityTypeRegular` - Standard API gateway
- `FunctionalityTypeAI` - AI-specific gateway features
- `FunctionalityTypeEvent` - Event-driven gateway features

## Error Handling

The client maps HTTP status codes to domain-specific errors:

- `400 Bad Request` → `utils.ErrBadRequest`
- `401 Unauthorized` → `utils.ErrUnauthorized`
- `403 Forbidden` → `utils.ErrForbidden`
- `404 Not Found` → Context-specific error (e.g., `ErrGatewayNotFound`)
- `409 Conflict` → Context-specific error (e.g., `ErrGatewayAlreadyExists`)
- `500 Internal Server Error` → `utils.ErrServiceUnavailable`

## Retry Logic

The client includes automatic retry logic for transient failures:

- Exponential backoff between retries
- Configurable max retries and wait times
- Only retries safe HTTP methods (GET, HEAD, OPTIONS)
- Respects `Retry-After` headers

## Type Conversions

The client handles conversions between:

1. **Generated types** (`gen.GatewayResponse`, etc.) - from OpenAPI spec
2. **Client types** (`client.GatewayResponse`, etc.) - simplified domain types
3. **Request types** - user-friendly input structures

All UUID conversions are handled automatically.

## Testing

Mock implementations can be generated using:

```bash
go generate ./...
```

This creates `clientmocks/apiplatform_client_fake.go` for testing.

## Authentication

The client uses an `AuthProvider` interface for flexible authentication:

### AuthProvider Interface

```go
type AuthProvider interface {
    GetToken(ctx context.Context) (string, error)
    InvalidateToken()
}
```

### OAuth2 Implementation

The default `auth.AuthProvider` implementation:
- Uses OAuth2 client credentials flow
- Automatically caches access tokens
- Refreshes tokens before expiry (30s buffer)
- Thread-safe token management

### Token Lifecycle

1. First request: `GetToken()` fetches a new token
2. Subsequent requests: Returns cached token if valid
3. Token near expiry: Automatically refreshes
4. 401 errors: Call `InvalidateToken()` to force refresh

## Comparison with OpenChoreo Client

This client follows the **exact same patterns** as `openchoreosvc/client`:

| Feature | OpenChoreo | API Platform |
|---------|-----------|--------------|
| Generated code directory | `gen/` | `gen/` |
| Wrapper client directory | `client/` | `client/` |
| Config structure | `client.Config` | `client.Config` |
| Authentication | `AuthProvider` interface | `AuthProvider` interface |
| Auth implementation | `openchoreosvc/auth` | `apiplatformsvc/auth` |
| OAuth2 client credentials | ✅ | ✅ |
| Token caching | ✅ | ✅ |
| Automatic refresh | ✅ | ✅ |
| Retry logic | ✅ | ✅ |
| Error mapping | ✅ | ✅ |
| Type conversions | ✅ | ✅ |
| Mock generation | ✅ | ✅ |

## Directory Structure

```
clients/apiplatformsvc/
├── auth/                      # Authentication provider
│   └── auth.go               # OAuth2 client credentials implementation
├── client/                    # Client wrapper
│   ├── auth.go               # AuthProvider interface
│   ├── client.go             # Main GatewayClient implementation
│   ├── constants.go          # Constants and enums
│   ├── errors.go             # Error handling
│   ├── types.go              # Request/response types
│   └── utils.go              # Helper functions
├── gen/                       # Generated code (do not edit)
│   ├── client.gen.go
│   ├── types.gen.go
│   ├── oapi-codegen.yaml
│   └── oapi-codegen-client.yaml
└── README.md
```

## Future Enhancements

- [ ] Add support for API Key authentication
- [ ] Add support for JWT file-based authentication
- [ ] Add response caching support
- [ ] Add metrics and observability
- [ ] Add more gateway artifact operations (APIs, deployments)
- [ ] Add API and API Product operations
