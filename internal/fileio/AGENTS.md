# fileio

Shared regular-file and directory admission for readers and config. No SDK dependencies.
- Use typed OpenRegular/OpenDirectory admission; reject the wrong kind and FIFO substitutions without blocking. Verify the opened descriptor matches the pre-open identity.
- Keep OS flags in platform files. Callers own canonicalization, authorization, byte limits and closing descriptors.
- `SyncDirectory` and `NonblockFlag` are the only definitions of directory durability and the nonblocking open flag; do not copy them into a package. On Windows a directory sync is a no-op.
