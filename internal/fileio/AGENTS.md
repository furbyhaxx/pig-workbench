# fileio

Shared regular-file and directory admission for readers and config. No SDK dependencies.
- Use typed OpenRegular/OpenDirectory admission; reject the wrong kind and FIFO substitutions without blocking. Verify the opened descriptor matches the pre-open identity.
- Keep OS flags in platform files. Callers own canonicalization, authorization, byte limits and closing descriptors.
- `SyncDirectory` and `NonblockFlag` are the only definitions of directory durability and the nonblocking open flag; do not copy them into a package. On Windows a directory sync is a no-op.
- `WriteAtomic` is the replace-by-rename writer for private state (config, endpoint records, project registry, storage journal). `fileedit` keeps its own two-phase commit and create-exclusive writers (`os.Link`) stay local: they are different contracts.
- `Lock` takes an exclusive private lock file and proves the held file is still the one at the path (`ErrLockReplaced` otherwise). Shared locks and non-waiting sweeps stay with their callers.
