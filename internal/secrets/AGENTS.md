# secrets

SPEC-0013 §3 defines deterministic placeholders and key handling; this package is independent of the SDK.
- `key.go` owns key-file creation and loading; never replace an existing key or silently relax its permissions.
- Derive 32-byte HMAC keys with HKDF-SHA256, no salt, info `nukii red-sec hmac v1`; placeholders use HMAC-SHA256 and the first 16 lowercase unpadded RFC 4648 base32 characters.
- Never include key or secret bytes in errors; withhold output on any redaction error. Preserve existing placeholders during replacement.
- `vault.go` and `vault_io.go` own private append-only AES-256-GCM storage, typed errors and project-before-global environment selection. The first ciphertext for a placeholder wins; metadata and deletion use the last record.
- `Load` and mutations reread under the sidecar lock; `Value` and `Values` call `Load`. Check `Warnings()` after `Load()` for an incomplete final line; writes fail until that line is repaired. `Values()` can return partial values with `ErrDecrypt`; redaction boundaries must withhold output on any error.
- Token detection and host adapters are separate concerns and are not implemented by this package.
