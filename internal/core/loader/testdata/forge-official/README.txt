# Fixed official installer metadata

These JSON projections were read from the exact official Forge installer URLs
listed in samples.json. Each installer was fetched into memory only, bounded to
32 MiB; its SHA-1 matched the independently requested official .sha1 sidecar.
The ledger records the observed size, SHA-1, SHA-256 and exact coordinate.

The projections retain original parsing fields and values. Display text, icons,
logos, mirror lists, comments and other unused UI fields were omitted. The
projection hashes identify the checked-in JSON, not the original ZIP entry.
No JAR, class, logo, skin, executable or installer implementation is redistributed.
No Java, installer or game was executed during collection.

The samples cover install/versionInfo (1.7.10), embedded Maven spec 0 with empty
processors (1.12.2 builds 2859 and 2860), and client processor spec 1 (1.20.4).
Version numbers and IDs were not replaced to manufacture an older generation.

Metadata-only tests may put placeholder bytes at an embedded Maven path to
exercise structural checks. Such bytes do not match the official artifact hash
and are not evidence of extraction, installation, linkage or a playable game.
Any full-pipeline synthetic payload fixture is labeled separately.
