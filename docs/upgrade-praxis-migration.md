# Upgrade: Operator-managed OGX → Praxis migration (3.5 → 3.6)

This document describes the operator-managed lifecycle for migrating Responses and
Conversations state from an OGX-centered 3.5 deployment to a Praxis-fronted 3.6 topology.

## Scope

| State | Owner after migration |
|-------|------------------------|
| Responses / Conversations / conversation items | Migrated into Praxis-compatible tables |
| Files / Vector Stores / ingestion / RAG resource tables | Remain OGX-owned (untouched) |

Migration is **opt-in**. CRs that never set `spec.praxisMode.migrationJob` (and have no prior
migration history) are status-unaffected: the operator does not create a Job and does not
write `status.migration` or migration conditions.

## CLI

The Job runs the OGX CLI ([#233](https://github.com/opendatahub-io/ogx/pull/233)):

```bash
ogx migrate praxis --dry-run "$OGX_CONFIG"
ogx migrate praxis "$OGX_CONFIG"
```

- Config: operator-generated ConfigMap mounted at `/etc/ogx/config.yaml` when the server
  would mount one; otherwise the image's `/etc/ogx/config.yaml`.
- Source DB: `spec.storage.sql.connectionString` (OGX Postgres Secret).
- Target DB: `spec.praxisMode.migrationJob.targetConnectionString` (required Praxis Postgres Secret).
- Credentials are injected via `SecretKeyRef` only.
- Job success is CLI exit 0. The CLI may skip unconfigured store phases with warnings and still exit 0.

## Orchestration

```
[never opted in, no history] → no status.migration, no migration conditions, no Job

[opted in]
  → Preflight
       fail → PreflightFailed (no Job, cutover blocked)
       pass → create/track Job {name}-praxis-migration
  → Running (Job Active / Pending / still terminating after a replace)
  → Failed (Job condition type=Failed; Job kept for logs; cutover blocked)
  → Validated (Job condition type=Complete)
       → PraxisCutoverReady=True
```

| Condition | Meaning |
|-----------|---------|
| `MigrationPreflightReady` | OGX Postgres Secret exists (`spec.storage.sql`, `ogx.io/watch=true`); Praxis `targetConnectionString` Secret exists and is a different database than the OGX source; ConfigMap if the operator mounts one; CA bundle ConfigMap if TLS trust is configured |
| `MigrationJobSucceeded` | Kubernetes Job reached condition `Complete` |
| `MigrationValidated` | Same Job: CLI dry-run and live migrate exited 0 |
| `PraxisCutoverReady` | Cutover gate — only True after `MigrationValidated` |
| `SoftRollbackAvailable` | Soft rollback only; includes data-loss warning |

`PraxisCutoverReady` is independent of DeploymentReady / HealthCheck. It is not a Praxis
startup-guard or post-write inventory check.

## Idempotency and retries

- Job name is `<ogxserver-name>-praxis-migration`, owned by the OGXServer.
- Attempt fingerprint: OGX (and Praxis, if distinct) Secret **content**, mounted ConfigMap
  content when present, and image.
- Same in-flight, succeeded, or failed attempt is not duplicated. A Failed Job is kept so
  logs remain: `oc logs job/<ogxserver-name>-praxis-migration`.
- Pod retries only (`backoffLimit=2`). After terminal Job `Failed`, delete the Job to retry.
  Changing Secret/config/image content also starts a new attempt. Stale Jobs are deleted
  with foreground propagation so replacement waits until dependent Pods are gone.
- A Job with this name that is not owned by the OGXServer is left untouched and blocks
  migration until it is deleted.
- Do not delete a succeeded Job; that attempt stays Validated and is not recreated.

The Job pod uses the same ServiceAccount override, FSGroup, and container resources as the
server workload.

## Write freeze

There is no OGXServer API to freeze Responses/Conversations writers. Apply an operational
write freeze before enabling the migration Job. The operator-enforced gate is
`PraxisCutoverReady`.

## Soft rollback

Full DB rollback is not supported. Soft rollback returns to the pre-enablement OGX serving
posture (typically `spec.praxisMode.enabled: false`). Responses/Conversations writes made
through Praxis after cutover do not flow back to OGX, ABAC flattening is not restored, and
data loss is possible. The validating webhook emits an admission warning when migration is
disabled after it was opted in (`praxisMode.enabled: false`, `migrationJob.enabled: false`,
or removing `migrationJob`). Job create / fail / cutover still emit Kubernetes Events.


## Example

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: demo
spec:
  distribution:
    name: starter
  storage:
    sql:
      type: postgres
      connectionString:
        name: ogx-pg
        key: url
  praxisMode:
    enabled: true
    migrationJob:
      enabled: true
      targetConnectionString:
        name: praxis-pg
        key: url
```

The OGX Postgres Secret (`spec.storage.sql.connectionString`) and the Praxis
`targetConnectionString` Secret are both required and must carry `ogx.io/watch: "true"`.
Do not point `targetConnectionString` at the OGX database: both default schemas include
`openai_conversations`. Preflight rejects the same Secret key or identical connection-string
bytes. A user ConfigMap is not required: the Job uses the
operator-generated config when the server would mount one, otherwise the image's
`/etc/ogx/config.yaml`. When TLS trust (or an ODH trusted CA bundle) is configured, the
Job mounts the same CA bundle as the server and sets `SSL_CERT_FILE`. Disabling
`migrationJob` deletes the Job.
