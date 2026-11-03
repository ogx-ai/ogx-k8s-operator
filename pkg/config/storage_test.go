/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"gopkg.in/yaml.v3"
)

// odhRelease36StorageBase is the ODH 3.6 product storage shape: kv_postgres +
// sql_postgres backends and the shipped store mappings (including queue settings,
// connectors, and vector_store_metadata).
const odhRelease36StorageBase = `version: '2'
apis:
- inference
- responses
- conversations
- vector_io
providers:
  vector_io:
  - provider_id: faiss
    provider_type: inline::faiss
    config: {}
storage:
  backends:
    kv_default:
      type: kv_postgres
      host: ${env.POSTGRES_HOST:=localhost}
      port: ${env.POSTGRES_PORT:=5432}
      db: ${env.POSTGRES_DB:=ogx}
      user: ${env.POSTGRES_USER:=ogx}
      password: ${env.POSTGRES_PASSWORD:=ogx}
      table_name: ${env.POSTGRES_TABLE_NAME:=ogx_kvstore}
    sql_default:
      type: sql_postgres
      host: ${env.POSTGRES_HOST:=localhost}
      port: ${env.POSTGRES_PORT:=5432}
      db: ${env.POSTGRES_DB:=ogx}
      user: ${env.POSTGRES_USER:=ogx}
      password: ${env.POSTGRES_PASSWORD:=ogx}
    kv_cache:
      type: kv_redis
      host: redis:6379
  extra_field: keep-me
  stores:
    metadata:
      backend: kv_default
      namespace: registry
    inference:
      table_name: inference_store
      backend: sql_default
      max_write_queue_size: 10000
      num_writers: 4
    conversations:
      table_name: openai_conversations
      backend: sql_default
    prompts:
      namespace: prompts
      backend: kv_default
    connectors:
      table_name: connectors
      backend: sql_default
    vector_stores:
      table_name: vector_store_metadata
      backend: sql_default
server:
  port: 8321
`

func TestGenerateConfig_SQLOnlyPraxisInheritsBaseKVAndStores(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Storage: &ogxiov1beta1.StateStorageSpec{
			SQL: &ogxiov1beta1.SQLStorageSpec{
				Type: "postgres",
				ConnectionString: &ogxiov1beta1.SecretKeyRef{
					Name: "ogx-pg",
					Key:  "url",
				},
			},
		},
	}

	generated, err := GenerateConfig(spec, []byte(odhRelease36StorageBase), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	storage := parseGeneratedStorage(t, generated.ConfigYAML)
	kv := requireStorageMap(t, requireStorageMap(t, storage, "backends"), "kv_default")
	assertConfigValues(t, kv, map[string]interface{}{
		"type":       "kv_postgres",
		"host":       "${env.POSTGRES_HOST:=localhost}",
		"port":       "${env.POSTGRES_PORT:=5432}",
		"db":         "${env.POSTGRES_DB:=ogx}",
		"user":       "${env.POSTGRES_USER:=ogx}",
		"password":   "${env.POSTGRES_PASSWORD:=ogx}",
		"table_name": "${env.POSTGRES_TABLE_NAME:=ogx_kvstore}",
	})

	sql := requireStorageMap(t, requireStorageMap(t, storage, "backends"), "sql_default")
	assertConfigValues(t, sql, map[string]interface{}{
		"type":              "sql_postgres",
		"connection_string": "${env.OGX_STORAGE_SQL_CONNECTION_STRING}",
	})
	assertConfigAbsent(t, sql, "host")
	assertConfigAbsent(t, sql, "port")
	assertConfigAbsent(t, sql, "db")
	assertConfigAbsent(t, sql, "user")
	assertConfigAbsent(t, sql, "password")

	assertConfigValue(t, storage, "extra_field", "keep-me")
	assertConfigValues(t, requireStorageMap(t, requireStorageMap(t, storage, "backends"), "kv_cache"), map[string]interface{}{
		"type": "kv_redis",
		"host": "redis:6379",
	})

	stores := requireStorageMap(t, storage, "stores")
	assertConfigValues(t, requireStorageMap(t, stores, "metadata"), map[string]interface{}{
		"backend":   "kv_default",
		"namespace": "registry",
	})
	assertConfigValues(t, requireStorageMap(t, stores, "inference"), map[string]interface{}{
		"backend":              "sql_default",
		"table_name":           "inference_store",
		"max_write_queue_size": 10000,
		"num_writers":          4,
	})
	assertConfigValues(t, requireStorageMap(t, stores, "conversations"), map[string]interface{}{
		"backend":    "sql_default",
		"table_name": "openai_conversations",
	})
	assertConfigValues(t, requireStorageMap(t, stores, "prompts"), map[string]interface{}{
		"backend":   "kv_default",
		"namespace": "prompts",
	})
	assertConfigValues(t, requireStorageMap(t, stores, "connectors"), map[string]interface{}{
		"backend":    "sql_default",
		"table_name": "connectors",
	})
	assertConfigValues(t, requireStorageMap(t, stores, "vector_stores"), map[string]interface{}{
		"backend":    "sql_default",
		"table_name": "vector_store_metadata",
	})

	apis, present := parseAPIList(t, generated.ConfigYAML)
	if !present {
		t.Fatalf("expected apis list, got:\n%s", generated.ConfigYAML)
	}
	assertAPIListOmits(t, apis, generated.ConfigYAML, "responses", "conversations")
}

func TestGenerateConfig_KVOnlyInheritsBaseSQL(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Storage: &ogxiov1beta1.StateStorageSpec{
			KV: &ogxiov1beta1.KVStorageSpec{Type: "sqlite"},
		},
	}

	generated, err := GenerateConfig(spec, []byte(odhRelease36StorageBase), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	storage := parseGeneratedStorage(t, generated.ConfigYAML)
	kv := requireStorageMap(t, requireStorageMap(t, storage, "backends"), "kv_default")
	assertConfigValues(t, kv, map[string]interface{}{
		"type":    "kv_sqlite",
		"db_path": "${env.SQLITE_STORE_DIR:=/.ogx}/kvstore.db",
	})
	assertConfigAbsent(t, kv, "host")
	assertConfigAbsent(t, kv, "table_name")

	sql := requireStorageMap(t, requireStorageMap(t, storage, "backends"), "sql_default")
	assertConfigValues(t, sql, map[string]interface{}{
		"type":     "sql_postgres",
		"host":     "${env.POSTGRES_HOST:=localhost}",
		"password": "${env.POSTGRES_PASSWORD:=ogx}",
	})

	stores := requireStorageMap(t, storage, "stores")
	assertConfigValue(t, requireStorageMap(t, stores, "connectors"), "table_name", "connectors")
	assertConfigValue(t, requireStorageMap(t, stores, "vector_stores"), "table_name", "vector_store_metadata")
}

func distributionStorageBase() map[string]interface{} {
	return map[string]interface{}{
		"backends": map[string]interface{}{
			"kv_default": map[string]interface{}{
				"type":       "kv_postgres",
				"host":       "${env.POSTGRES_HOST:=localhost}",
				"port":       "${env.POSTGRES_PORT:=5432}",
				"db":         "${env.POSTGRES_DB:=ogx}",
				"user":       "${env.POSTGRES_USER:=ogx}",
				"password":   "${env.POSTGRES_PASSWORD:=ogx}",
				"table_name": "${env.POSTGRES_TABLE_NAME:=ogx_kvstore}",
			},
			"sql_default": map[string]interface{}{
				"type":     "sql_postgres",
				"host":     "${env.POSTGRES_HOST:=localhost}",
				"password": "${env.POSTGRES_PASSWORD:=ogx}",
			},
			"kv_cache": map[string]interface{}{"type": "kv_redis", "host": "redis:6379"},
		},
		"stores": map[string]interface{}{
			"metadata":      map[string]interface{}{"backend": "kv_default", "namespace": "registry"},
			"inference":     map[string]interface{}{"backend": "sql_default", "table_name": "inference_store", "num_writers": 4},
			"conversations": map[string]interface{}{"backend": "sql_default", "table_name": "openai_conversations"},
			"prompts":       nil,
			"connectors":    map[string]interface{}{"backend": "sql_default", "table_name": "connectors"},
			"vector_stores": map[string]interface{}{"backend": "sql_default", "table_name": "vector_store_metadata"},
		},
		"extra_field": "keep-me",
	}
}

func TestMergeStorage_MissingBaseFallsBackToSQLite(t *testing.T) {
	user := storageOverlay(&ogxiov1beta1.StateStorageSpec{
		SQL: &ogxiov1beta1.SQLStorageSpec{Type: "postgres"},
	})
	result := MergeStorage(nil, user)
	kv := requireStorageMap(t, requireStorageMap(t, result, "backends"), "kv_default")
	if kv["type"] != "kv_sqlite" {
		t.Errorf("expected sqlite KV fallback, got %v", kv["type"])
	}
	sql := requireStorageMap(t, requireStorageMap(t, result, "backends"), "sql_default")
	if sql["type"] != "sql_postgres" {
		t.Errorf("expected postgres SQL overlay, got %v", sql["type"])
	}
	stores := requireStorageMap(t, result, "stores")
	if _, ok := stores["metadata"]; !ok {
		t.Error("expected default metadata store mapping when base is missing")
	}
}

func TestMergeStorage_ExplicitSQLiteReplacesPostgresWithoutStaleFields(t *testing.T) {
	user := storageOverlay(&ogxiov1beta1.StateStorageSpec{
		KV: &ogxiov1beta1.KVStorageSpec{Type: "sqlite"},
	})
	result := MergeStorage(distributionStorageBase(), user)
	kv := requireStorageMap(t, requireStorageMap(t, result, "backends"), "kv_default")
	assertConfigValues(t, kv, map[string]interface{}{
		"type":    "kv_sqlite",
		"db_path": "${env.SQLITE_STORE_DIR:=/.ogx}/kvstore.db",
	})
	for _, stale := range []string{"host", "port", "db", "user", "password", "table_name"} {
		assertConfigAbsent(t, kv, stale)
	}
}

func TestMergeStorage_ExplicitRedisReplacesPostgresWithoutStaleFields(t *testing.T) {
	user := storageOverlay(&ogxiov1beta1.StateStorageSpec{
		KV: &ogxiov1beta1.KVStorageSpec{Type: "redis", Endpoint: "redis://my-redis:6379"},
	})
	result := MergeStorage(distributionStorageBase(), user)
	kv := requireStorageMap(t, requireStorageMap(t, result, "backends"), "kv_default")
	assertConfigValues(t, kv, map[string]interface{}{
		"type": "kv_redis",
		"host": "redis://my-redis:6379",
	})
	for _, stale := range []string{"port", "db", "user", "password", "table_name", "db_path"} {
		assertConfigAbsent(t, kv, stale)
	}
}

func TestMergeStorage_NilStorageClonesBaseWithoutFillingDefaults(t *testing.T) {
	baseKV := requireStorageMap(t, requireStorageMap(t, distributionStorageBase(), "backends"), "kv_default")
	base := map[string]interface{}{
		"backends": map[string]interface{}{"kv_default": cloneMap(baseKV)},
		"stores":   map[string]interface{}{"metadata": map[string]interface{}{"backend": "kv_default"}},
	}
	result := MergeStorage(base, nil)
	stores := requireStorageMap(t, result, "stores")
	if _, ok := stores["prompts"]; ok {
		t.Error("nil storage must not fill default store mappings")
	}
	if _, ok := requireStorageMap(t, result, "backends")["sql_default"]; ok {
		t.Error("nil storage must not add sqlite SQL fallback")
	}
	if got := requireStorageMap(t, requireStorageMap(t, result, "backends"), "kv_default")["type"]; got != "kv_postgres" {
		t.Errorf("expected base KV to be unchanged, got %v", got)
	}
}

func TestMergeStorage_PreservesExplicitNullStoreMappings(t *testing.T) {
	user := storageOverlay(&ogxiov1beta1.StateStorageSpec{
		SQL: &ogxiov1beta1.SQLStorageSpec{Type: "postgres"},
	})
	result := MergeStorage(distributionStorageBase(), user)
	stores := requireStorageMap(t, result, "stores")
	if _, present := stores["prompts"]; !present {
		t.Fatal("expected prompts key to remain")
	}
	if stores["prompts"] != nil {
		t.Errorf("expected explicit null prompts mapping to be preserved, got %v", stores["prompts"])
	}
	assertConfigValue(t, requireStorageMap(t, stores, "inference"), "num_writers", 4)
	assertConfigValue(t, requireStorageMap(t, stores, "connectors"), "table_name", "connectors")
	assertConfigValue(t, result, "extra_field", "keep-me")
}

func TestMergeStorage_DoesNotMutateBase(t *testing.T) {
	base := distributionStorageBase()
	delete(requireStorageMap(t, base, "stores"), "vector_stores")
	originalStoresLen := len(requireStorageMap(t, base, "stores"))
	originalHost := requireStorageMap(t, requireStorageMap(t, base, "backends"), "kv_default")["host"]

	user := storageOverlay(&ogxiov1beta1.StateStorageSpec{
		SQL: &ogxiov1beta1.SQLStorageSpec{Type: "postgres"},
	})
	result := MergeStorage(base, user)
	applyPraxisVectorStoreMetadata(result, true)

	if _, declared := requireStorageMap(t, result, "stores")["vector_stores"]; !declared {
		t.Fatal("expected Praxis to declare vector_stores on the merged copy")
	}
	if _, declared := requireStorageMap(t, base, "stores")["vector_stores"]; declared {
		t.Error("Praxis must not mutate base stores")
	}
	if len(requireStorageMap(t, base, "stores")) != originalStoresLen {
		t.Errorf("base stores length changed: got %d want %d", len(requireStorageMap(t, base, "stores")), originalStoresLen)
	}
	requireStorageMap(t, requireStorageMap(t, result, "backends"), "kv_default")["host"] = "hacked"
	if got := requireStorageMap(t, requireStorageMap(t, base, "backends"), "kv_default")["host"]; got != originalHost {
		t.Errorf("mutating merged KV must not change base, got %v", got)
	}
}

func TestGenerateConfig_NilStorageLeavesBaseUnchanged(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{}
	generated, err := GenerateConfig(spec, []byte(odhRelease36StorageBase), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	storage := parseGeneratedStorage(t, generated.ConfigYAML)
	kv := requireStorageMap(t, requireStorageMap(t, storage, "backends"), "kv_default")
	assertConfigValue(t, kv, "type", "kv_postgres")
	sql := requireStorageMap(t, requireStorageMap(t, storage, "backends"), "sql_default")
	assertConfigValue(t, sql, "type", "sql_postgres")
	assertConfigAbsent(t, sql, "connection_string")
	assertConfigValue(t, requireStorageMap(t, requireStorageMap(t, storage, "stores"), "vector_stores"), "table_name", "vector_store_metadata")
}

func parseGeneratedStorage(t *testing.T, configYAML string) map[string]interface{} {
	t.Helper()
	var cfg map[string]interface{}
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}
	storage, ok := cfg["storage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected storage map, got %v in:\n%s", cfg["storage"], configYAML)
	}
	return storage
}

func requireStorageMap(t *testing.T, parent map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	value, ok := parent[key].(map[string]interface{})
	if !ok {
		t.Fatalf("expected %s map, got %v", key, parent[key])
	}
	return value
}
