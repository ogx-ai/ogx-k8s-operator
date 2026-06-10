# Config Audit: Operator CRD vs Upstream StackConfig

**Current status:** all P0 and P1 issues have been fixed. #19 (add `file_processors` API) has been addressed

**Date:** 2026-06-09
**Upstream ref:** `ogx-ai/ogx@main` — `src/ogx/core/datatypes.py` (`StackConfig`)
**Product build ref:** `opendatahub-io/ogx-distribution@main` — `distribution/config.yaml`
**Operator branch:** `pr/VaishnaviHire/295`

---

## P0 — Config field name mismatches (generated config will not work)

### 1. `url` should be `base_url` for vLLM, OpenAI, Azure, WatsonX

In `pkg/config/provider.go:138`, `:160`, `:173`, `:260` — the operator generates `"url"` but the upstream provider configs define the field as `base_url` (confirmed in `VLLMInferenceAdapterConfig`, `AzureConfig`, and inherited via `RemoteInferenceProviderConfig`).

| Provider | Operator generates | Upstream expects |
|----------|--------------------|------------------|
| vLLM     | `url`              | `base_url`       |
| OpenAI   | `url`              | `base_url` (via parent) |
| Azure    | `url`              | `base_url`       |
| WatsonX  | `url`              | `base_url` (via parent) |

### 2. Bedrock `region` should be `region_name`

In `pkg/config/provider.go:193` — operator generates `"region"` but `BedrockConfig` defines the field as `region_name`.

### 3. Network config nesting is wrong — `tls`/`timeout`/`headers` should be under `network`

In `pkg/config/provider.go:566-610` — `applyNetworkConfig` puts `tls`, `timeout`, and `headers` at the **top level** of the provider config dict. The upstream `RemoteInferenceProviderConfig` expects these nested under `network`:

```yaml
# Operator generates (WRONG):        # Upstream expects (CORRECT):
config:                               config:
  tls:                                  network:
    verify: true                          tls:
  timeout:                                  verify: true
    connect: 30                           timeout:
  headers: {...}                            connect: 30
                                          headers: {...}
```

---

## P1 — CRD fields silently dropped (users set them, nothing happens)

### 4. `refreshModels` and `allowedModels` not generated for any inference provider

These are valid fields on `RemoteInferenceProviderConfig` (`refresh_models: bool`, `allowed_models: list[str]`). The CRD exposes them on every inference provider type, but `pkg/config/provider.go` never includes them in the generated config. Users who set these fields get no effect.

### 5. Qdrant — 8 CRD fields silently dropped

`pkg/config/provider.go:363-376` — only `url` and `api_key` are generated. These CRD fields are ignored:

| CRD field    | Upstream config key |
|--------------|---------------------|
| `host`       | `host`              |
| `port`       | `port`              |
| `grpcPort`   | `grpc_port`         |
| `preferGrpc` | `prefer_grpc`       |
| `https`      | `https`             |
| `prefix`     | `prefix`            |
| `timeout`    | `timeout`           |
| `location`   | `location`          |

### 6. Milvus `consistencyLevel` not generated

`pkg/config/provider.go:348-361` — the CRD has `consistencyLevel` but only `uri` and `token` are generated.

### 7. PGVector `distanceMetric` and `vectorIndex` not generated

`pkg/config/provider.go:325-346` — CRD has `distanceMetric` (enum) and full `vectorIndex` config (HNSW/IVFFlat params), none generated.

### 8. Proxy config never applied

`applyNetworkConfig` in `pkg/config/provider.go:566-575` calls `applyTLSConfig`, `applyTimeoutConfig`, and handles `headers`, but never calls anything for `network.Proxy`. The CRD has a full `ProxyConfig` struct (`url`, `http`, `https`, `cacert`, `noProxy`) that is completely ignored.

### 9. Responses provider: `vectorStoresConfig` and `compactionConfig` not generated

`pkg/config/provider.go:558-564` — `expandBuiltinResponsesProvider` returns an empty config. The CRD has rich `VectorStoresConfig` and `CompactionConfig` types that are silently dropped.

### 10. File search provider: `vectorStoresConfig` not generated

`pkg/config/provider.go:443-451` — `expandInlineFileSearchProvider` returns empty config, ignoring the CRD's `VectorStoresConfig`.

### 11. Batches reference provider: config fields not generated

`pkg/config/provider.go:532-538` — `expandReferenceProvider` returns empty config. CRD fields `maxConcurrentBatches` and `maxConcurrentRequestsPerBatch` are dropped.

### 12. Model `quantization` not generated

`pkg/config/generator.go:149-165` — `serializeModels` includes `model_id`, `provider_id`, `model_type`, `context_length` but not `quantization`.

---

## P2 — Missing StackConfig features (no CRD representation)

### 13. `server.auth` — No authentication configuration

The upstream `ServerConfig.auth` supports 5 auth provider types (OAuth2, GitHub, Custom, Kubernetes, UpstreamHeader) plus `route_policy` and `access_policy`. The product build configures OAuth2 auth. The operator CRD has no auth fields at all. Users must rely on `baseConfig`/`overrideConfig` for auth.

### 14. `server.workers` not mapped from CRD

`pkg/config/generator.go:179-190` — `buildServerSection` only maps `port`. The CRD has `workload.workers` but it is not injected into the server config's `workers` field.

### 15. `server.registry_refresh_interval_seconds` not exposed

No CRD field for this. Defaults to 300s upstream.

### 16. `server.host` not exposed

No CRD field for binding to a specific host/interface.

### 17. `logging` / telemetry not exposed

The upstream `StackConfig.logging` (`LoggingConfig`) and the product build's `telemetry` section have no CRD representation.

### 18. `connectors` not exposed

`StackConfig.connectors: list[ConnectorInput]` has no CRD representation.

### 19. `file_processors` API/provider not handled

The product build includes `file_processors` in its API list and a `pypdf` provider. The operator doesn't have a `file_processors` provider expander.

---

## P2b — Storage gaps

### 20. `kv_postgres` backend not supported

The product build uses `kv_postgres` for its KV backend (individual host/port/db/user/password fields). The operator's KV storage only supports `sqlite` and `redis` (`pkg/config/storage.go:53-67`). When users rely on the product's base config, the KV postgres config passes through unchanged, but they can't configure it via the CRD.

### 21. SQL postgres uses `connection_string` but upstream uses individual fields

The operator generates `connection_string` (`pkg/config/storage.go:72-76`), but `PostgresSqlStoreConfig` uses `host`, `port`, `db`, `user`, `password`, `pool_size`, `max_overflow`, `pool_recycle`. These are incompatible schemas — `connection_string` is not a field on the upstream class. (Needs verification — there may be an adapter that accepts connection strings.)

### 22. Missing stores: `connectors`, `responses`, `vector_stores`

`pkg/config/storage.go:95-114` — `defaultStores` generates 4 stores (metadata, inference, conversations, prompts). The upstream `ServerStoresConfig` also defines `connectors` (with a default), `responses`, and `vector_stores`.

---

## P3 — Missing typed providers (available via CustomProvider)

### 23. Gemini (`remote::gemini`)

In the product build but no typed CRD provider. Users must use `CustomProvider`.

### 24. Anthropic (`remote::anthropic`)

Same as above.

---

## Summary

| Severity | Count | Impact |
|----------|-------|--------|
| P0 — Wrong field names            | 3 issues  | Generated config will be rejected or silently ignored by the server |
| P1 — Silently dropped CRD fields  | 9 issues  | Users set values that have no effect |
| P2 — Missing features             | 10 issues | Functionality not configurable via CRD |
| P3 — Missing typed providers       | 2 issues  | Workaround via CustomProvider exists |

The P0 field name mismatches (`url` -> `base_url`, `region` -> `region_name`, flat TLS -> nested `network`) are the most urgent — they mean generated configs for these providers won't work correctly when the operator overrides the base config's providers.
