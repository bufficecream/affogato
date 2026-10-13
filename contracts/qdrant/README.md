# Qdrant configuration

Source of truth: [`docs/architecture-review.md`](../../docs/architecture-review.md) B3. The
Helm init Job (H1) applies these files; `contracts/check-config.sh` proves them against a real
Qdrant.

| File | Shape | Apply (idempotent) |
|---|---|---|
| `<collection>.json` | `collection_name`; `collection` = body of `PUT /collections/{collection_name}`; `payload_indexes[]` = bodies of `PUT /collections/{collection_name}/index` | create only if `GET /collections/{name}/exists` is false (a second `PUT` returns 409), then `PUT` each index with `?wait=true` (repeatable) |
| `aliases.json` | body of `POST /collections/aliases` | repeatable: `create_alias` on an existing alias re-points it |

- One physical collection per embedding model; `collection.metadata.embedding_model` records
  the model so a worker can check what the alias `transcripts` resolves to.
- HNSW, optimizer and WAL settings are left at the server defaults (B3 "default HNSW").
- **Model change:** add a new `transcripts_<model>_v<n>.json`, re-embed into it, then point
  `aliases.json` at it. Applying it swaps the alias in one request (`create_alias` re-points an
  existing alias; checked on Qdrant 1.19.2).
