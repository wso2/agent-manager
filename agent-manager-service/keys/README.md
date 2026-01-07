# JWT Signing Keys

This directory contains RSA key pairs used for signing and verifying agent API tokens.

## Files

- `private.pem` - RSA private key (2048-bit) used for signing JWTs
- `public.pem` - RSA public key used for verifying JWTs

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

## Key Rotation

To rotate keys:
1. Generate new keys with a different key ID
2. Update the `JWT_SIGNING_ACTIVE_KEY_ID` environment variable
3. Keep old keys available for a transition period to verify existing tokens
4. Remove old keys after all tokens signed with them have expired

## Configuration

Configure key paths in `.env`:
```bash
JWT_SIGNING_PRIVATE_KEY_PATH=./keys/private.pem
JWT_SIGNING_PUBLIC_KEY_PATH=./keys/public.pem
JWT_SIGNING_ACTIVE_KEY_ID=key-1
JWT_SIGNING_DEFAULT_EXPIRY=8760h  # 1 year
JWT_SIGNING_ISSUER=agent-manager-service
JWT_SIGNING_DEFAULT_ENVIRONMENT=development
```
