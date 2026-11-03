# fileio

Shared regular-file admission for readers and config. No SDK dependencies.
- Reject devices, directories and FIFO substitutions without blocking; verify the opened descriptor matches the pre-open identity.
- Keep OS flags in platform files. Callers own canonicalization, authorization, byte limits and closing descriptors.
