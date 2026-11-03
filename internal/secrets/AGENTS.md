# secrets

SPEC-0013 §3 defines deterministic placeholders and key handling; this package is independent of the SDK.
- `key.go` owns key-file creation and loading; never replace an existing key or silently relax its permissions.
- Derive 32-byte HMAC keys with HKDF-SHA256, no salt, info `nukii red-sec hmac v1`; placeholders use HMAC-SHA256 and the first 16 lowercase unpadded RFC 4648 base32 characters.
- Never include key or secret bytes in errors; withhold output on any redaction error. Preserve existing placeholders during replacement.
- Token detection, encrypted vault storage, environment injection and host adapters are separate concerns and are not implemented by these primitives.
