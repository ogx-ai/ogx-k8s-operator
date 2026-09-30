# Runtime Config Generation from OGXServer CR

This guide explains how to use OGX runtime config generation directly from the `OGXServer` custom resource, without hand-writing a full `config.yaml`.

## Overview

The operator can generate server `config.yaml` from declarative CR fields:

- `spec.providers`
- `spec.resources`
- `spec.storage`
- `spec.disabledAPIs`

When this mode is active, the operator resolves a base config from `spec.baseConfig` when provided, otherwise from OCI image labels, merges your CR values, writes an immutable generated ConfigMap, and mounts it into the server pod.

With `spec.praxisMode.enabled: true`, the operator also generates a config when no declarative
fields are set. It preserves the distribution default while replacing `server.auth`, enabling
multi-tenancy, and applying an explicit `spec.network.port`.

## Precedence Rules

The operator supports two config modes:

1. **Override mode**: `spec.overrideConfig` points to a user-managed ConfigMap key.
2. **Generated mode**: declarative CR fields, or Praxis mode, generate `config.yaml`.

If `spec.overrideConfig` is set, it takes precedence over generated mode.
The mounted runtime config always comes from `spec.overrideConfig` or the
generated ConfigMap; `spec.baseConfig` is only an input to generation.

## Required Resource Labels and Namespace Scope

Any ConfigMap/Secret referenced by the CR must:

- Be in the **same namespace** as the `OGXServer`
- Have label `ogx.io/watch: "true"`

Example:

```yaml
metadata:
  labels:
    ogx.io/watch: "true"
```

This label allows the operator to watch changes and trigger reconciliation.

## How It Works

1. If `spec.baseConfig` is set, read that ConfigMap key as the base config.
2. Otherwise, resolve base config from OCI labels `com.ogx.distribution.default-config` and `com.ogx.config.<default-config-filename>` on the resolved distribution image.
3. Expand providers/resources/storage from CR spec.
4. Merge with base config:
   - providers: user values replace base per API type
   - models/resources: user values replace base models
   - storage: user value replaces base storage
   - APIs: base filtered by `disabledAPIs`
   - Praxis mode: use upstream-header auth and `server.tenancy.mode: multi`
5. Create immutable ConfigMap: `${ogxserver-name}-config-${contentHash}`.
6. Mount generated `config.yaml` to `/etc/ogx/config.yaml`.
7. Inject secret-backed environment variables for provider/storage credentials.
8. Roll deployment when referenced ConfigMaps/Secrets change.

## Minimal Declarative Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: openai-creds
  labels:
    ogx.io/watch: "true"
stringData:
  api-key: "<token>"
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: generated-config-sample
spec:
  distribution:
    name: starter
  providers:
    inference:
      remote:
        openai:
          - id: openai-primary
            apiKey:
              name: openai-creds
              key: api-key
  resources:
    models:
      - name: gpt-4o-mini
        provider: openai-primary
```

Apply a ready-to-use sample from this repository:

```bash
kubectl apply -f config/samples/example-with-generated-config.yaml
```

## `configgen` CLI

The `configgen` CLI runs the same generation pipeline outside Kubernetes.

```bash
configgen <ogxserver.yaml> -base <config.yaml> [-distributions-path distributions.json] [-output-config] [-validate]
```

Notes:

- If the CR uses `spec.baseConfig`, pass that file with `-base`.
- If the CR only sets `spec.distribution.name`, pass `-base` as well; image
  resolution for named distributions happens in the operator.
- Without `-validate`, the CLI does not apply Kubernetes CRD schema defaults.
- With `-validate`, the CLI additionally validates against the CRD schema,
  CEL rules, and webhook logic.
- `-validate` uses `distributions.json` to validate `spec.distribution.name`.

## Named Storage Backends (Postgres)

Use `spec.storage.backends` to configure OGX's structured `kv_postgres` and
`sql_postgres` backends. API fields use Kubernetes-style camelCase; generated
OGX config uses snake_case. Host, database, user, TLS, and path values accept
literal strings or OGX environment substitutions. For example, the shipped
`POSTGRES_*` convention works as shown below, and custom names such as
`${env.DATABASE_HOST}` work the same way. `port` accepts either an integer or
an environment substitution. Passwords are Secret references and are never
written into the generated config as plaintext.
Each password Secret is exposed through an operator-generated environment
variable scoped to its backend. If `password` is omitted, the generated backend
uses passwordless authentication; it does not read `POSTGRES_PASSWORD` itself.
Integer pool settings accept integers or OGX environment substitutions. For the
boolean SQL `pool_pre_ping`, set `poolPrePing` to a literal boolean or use
`poolPrePingEnv` with an OGX environment substitution.

If `stores` is omitted, the operator creates these mappings:

| Logical store | Backend family | Config |
| --- | --- | --- |
| `metadata` | KV | namespace `registry` |
| `inference` | SQL | table `inference_store` |
| `conversations` | SQL | table `openai_conversations` |
| `prompts` | SQL | table `prompts` |
| `connectors` | SQL | table `connectors` |

The KV and SQL selections are inferred only when exactly one backend of each
family is configured. Missing or multiple backends of either family produce a
config-generation error. An explicit `stores` map is complete: omitted store
entries are not filled in. Set `stores: {}` to request no logical stores.
Known stores are checked against their expected backend family, and every
mapping must reference a configured backend.
OGX's `StackConfig` supplies defaults for omitted store fields, so generated
explicit maps set every supported but omitted logical store to `null`. This
keeps a partial map complete after OGX validation; `stores: {}` therefore
disables all stores. Supported names are `metadata`, `inference`,
`conversations`, `responses`, `prompts`, `connectors`, and `vector_stores`.
In Praxis mode, include `vector_stores` explicitly when a configured `vector_io`
provider requires it; the operator's automatic Praxis mapping is used only when
the store map is omitted.

These defaults are an intentional change for the named form: `prompts` maps to
SQL and `connectors` is added. The deprecated `kv`/`sql` converter keeps its
existing KV-form prompt entry during the compatibility period; OGX currently
migrates that namespace-style entry to a SQL table while validating its config.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ogx-db
  labels:
    ogx.io/watch: "true"
stringData:
  password: "<database-password>"
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: generated-config-with-storage
spec:
  distribution:
    name: starter
  storage:
    backends:
      pg_kv:
        type: kv_postgres
        host: ${env.POSTGRES_HOST:=localhost}
        port: ${env.POSTGRES_PORT:=5432}
        db: ${env.POSTGRES_DB:=ogx}
        user: ${env.POSTGRES_USER:=ogx}
        password:
          name: ogx-db
          key: password
        tableName: ${env.POSTGRES_TABLE_NAME:=ogx_kvstore}
        poolSize: 5
        maxOverflow: 10
        commandTimeout: 30
      pg_sql:
        type: sql_postgres
        host: ${env.POSTGRES_HOST:=localhost}
        port: ${env.POSTGRES_PORT:=5432}
        db: ${env.POSTGRES_DB:=ogx}
        user: ${env.POSTGRES_USER:=ogx}
        password:
          name: ogx-db
          key: password
        poolSize: 10
        maxOverflow: 20
        poolRecycle: 3600
        poolPrePing: true
```

To customize or restrict the store set, provide the complete map explicitly:

```yaml
storage:
  backends:
    archive:
      type: sql_postgres
      host: ${env.ARCHIVE_HOST}
      port: 5432
      db: ogx_archive
      user: ogx
  stores:
    conversations:
      backend: archive
      tableName: conversations_v2
```

The deprecated `storage.kv` and `storage.sql` fields remain available for
compatibility with SQLite, Redis, and existing SQL DSN configurations. Do not
combine those fields with `backends` or `stores`. Legacy
`storage.sql.connectionString` remains in use by the Praxis migration Job as
its OGX source credential; that Job currently requires the legacy SQL form.
Keep it through a migration that needs the Job. After migration, move the
connection details into the structured backend fields and put the password in
a watched Secret. The new named form emits `host`, `port`, `db`, `user`, and
other OGX fields, and never emits the legacy `connection_string` property.
The operator does not resolve or parse an old DSN Secret to populate those
fields; copy the connection details into the structured values during migration.

If the entire `storage` block is omitted, the base config's storage is kept.

## Status and Conditions

When generation is active, status includes:

- `.status.configGeneration.configMapName`
- `.status.configGeneration.generatedAt`
- `.status.configGeneration.providerCount`
- `.status.configGeneration.resourceCount`
- `.status.configGeneration.configVersion`

Condition `ConfigGenerated` indicates current generation state:

- `True` with reason `ConfigGenerationSucceeded` when generation succeeds
- `False` with reasons like `ConfigGenerationFailed` or `ConfigGenerationInactive`

## Rollout Behavior

Pod template annotations include hashes for generated config and referenced inputs.
Changing any of these triggers rollout:

- Generated config content
- Managed CA bundle (if configured)
- Referenced Secret resource versions

## Troubleshooting

- **Config not generated**
  - Check `kubectl get ogxserver <name> -o yaml` for `ConfigGenerated` condition and messages.
- **No restart after editing Secret/ConfigMap**
  - Ensure referenced object has `ogx.io/watch: "true"` and same namespace as `OGXServer`.
- **Validation errors for generated env var names**
  - Check provider IDs and custom secretRef keys for collisions after normalization.
- **Hash collision/content mismatch error**
  - Operator detected existing generated ConfigMap name with different content; update CR and re-reconcile.
