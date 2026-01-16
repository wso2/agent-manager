# JWT Signing Keys

This directory contains RSA key pairs used for signing and verifying agent API tokens.

## Files

- `private.pem` - RSA private key (2048-bit) used for signing JWTs
- `public.pem` - RSA public key used for verifying JWTs
- `public-keys-config.example.json` - Example configuration for multi-key support

## Generation

Keys are automatically generated when running:
```bash
make run
```

Or manually with:
```bash
make gen-keys
```

Or directly:
```bash
bash scripts/gen_keys.sh
```

To generate additional keys for rotation (with custom key ID):
```bash
bash scripts/gen_keys.sh key-2
# This creates private-key-2.pem and public-key-2.pem
```

## Configuration Modes

Configure in `.env`:
```bash
# Private key for signing (only one at a time)
JWT_SIGNING_PRIVATE_KEY_PATH=./keys/private.pem
JWT_SIGNING_ACTIVE_KEY_ID=key-1

# Public keys configuration (serves all keys via JWKS)
JWT_SIGNING_PUBLIC_KEYS_CONFIG=./keys/public-keys-config.json

# Other settings
JWT_SIGNING_DEFAULT_EXPIRY=8760h
JWT_SIGNING_ISSUER=agent-manager-service
JWT_SIGNING_DEFAULT_ENVIRONMENT=development
```

Create `public-keys-config.json`:
```json
{
  "keys": [
    {
      "kid": "key-1",
      "algorithm": "RS256",
      "publicKeyPath": "./keys/public.pem",
      "description": "Current active signing key",
      "createdAt": "2026-01-01T00:00:00Z"
    },
    {
      "kid": "key-2",
      "algorithm": "RS256",
      "publicKeyPath": "./keys/public-key-2.pem",
      "description": "New key for rotation",
      "createdAt": "2026-01-13T00:00:00Z"
    }
  ]
}
```
