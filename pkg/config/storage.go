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
	"fmt"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
)

const (
	storageBackendsKey = "backends"
	storageStoresKey   = "stores"
	kvDefaultBackend   = "kv_default"
	sqlDefaultBackend  = "sql_default"
)

// ApplyStorage generates the storage section for config.yaml based on the spec.
// Returns nil if no storage is configured (base config storage is preserved).
// Standalone calls (no distribution base) still emit SQLite fallbacks and default
// store mappings for omitted KV/SQL. GenerateConfig passes the overlay from
// storageOverlay through MergeStorage so omitted backends inherit from the base.
func ApplyStorage(storage *ogxiov1beta1.StateStorageSpec) map[string]interface{} {
	return MergeStorage(nil, storageOverlay(storage))
}

// storageOverlay returns only backends explicitly set on the CR. Omitted KV/SQL
// are left unset so MergeStorage can inherit the corresponding base backend.
func storageOverlay(storage *ogxiov1beta1.StateStorageSpec) map[string]interface{} {
	if storage == nil {
		return nil
	}

	return map[string]interface{}{
		storageBackendsKey: storageBackendsFromSpec(storage),
	}
}

func storageBackendsFromSpec(storage *ogxiov1beta1.StateStorageSpec) map[string]interface{} {
	backends := make(map[string]interface{})
	if storage.KV != nil {
		backends[kvDefaultBackend] = expandKVBackend(storage.KV)
	}
	if storage.SQL != nil {
		backends[sqlDefaultBackend] = expandSQLBackend(storage.SQL)
	}
	return backends
}

func expandKVBackend(kv *ogxiov1beta1.KVStorageSpec) map[string]interface{} {
	switch kv.Type {
	case "redis":
		cfg := map[string]interface{}{
			"type": "kv_redis",
			"host": kv.Endpoint,
		}
		if kv.Password != nil {
			cfg["password"] = fmt.Sprintf("${env.%s_STORAGE_KV_PASSWORD}", envVarPrefix)
		}
		return cfg
	default: // sqlite
		return defaultKVBackend()
	}
}

func expandSQLBackend(sql *ogxiov1beta1.SQLStorageSpec) map[string]interface{} {
	switch sql.Type {
	case "postgres":
		return map[string]interface{}{
			"type":              "sql_postgres",
			"connection_string": fmt.Sprintf("${env.%s_STORAGE_SQL_CONNECTION_STRING}", envVarPrefix),
		}
	default: // sqlite
		return defaultSQLBackend()
	}
}

func defaultKVBackend() map[string]interface{} {
	return map[string]interface{}{
		"type":    "kv_sqlite",
		"db_path": "${env.SQLITE_STORE_DIR:=/.ogx}/kvstore.db",
	}
}

func defaultSQLBackend() map[string]interface{} {
	return map[string]interface{}{
		"type":    "sql_sqlite",
		"db_path": "${env.SQLITE_STORE_DIR:=/.ogx}/sqlstore.db",
	}
}

func defaultStores() map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"backend":   kvDefaultBackend,
			"namespace": "registry",
		},
		"inference": map[string]interface{}{
			"backend":    sqlDefaultBackend,
			"table_name": "inference_store",
		},
		"conversations": map[string]interface{}{
			"backend":    sqlDefaultBackend,
			"table_name": "openai_conversations",
		},
		"prompts": map[string]interface{}{
			"backend":   kvDefaultBackend,
			"namespace": "prompts",
		},
	}
}

// overlayStorage copies base storage and applies an explicit CR overlay.
// User backends replace the complete corresponding backend object. Omitted KV/SQL
// inherit the cloned base backend, with SQLite used only when both omit it.
func overlayStorage(base, user map[string]interface{}) map[string]interface{} {
	merged := cloneStorageMap(base)
	if merged == nil {
		merged = make(map[string]interface{})
	}

	overlayStorageBackends(merged, user)
	fillDefaultStores(merged)
	return merged
}

func overlayStorageBackends(merged, user map[string]interface{}) {
	backends := asStorageMap(merged[storageBackendsKey])
	if backends == nil {
		backends = make(map[string]interface{})
	}

	if userBackends := asStorageMap(user[storageBackendsKey]); userBackends != nil {
		if kv, ok := userBackends[kvDefaultBackend]; ok {
			backends[kvDefaultBackend] = cloneIfMap(kv)
		}
		if sql, ok := userBackends[sqlDefaultBackend]; ok {
			backends[sqlDefaultBackend] = cloneIfMap(sql)
		}
	}

	if _, ok := backends[kvDefaultBackend]; !ok {
		backends[kvDefaultBackend] = defaultKVBackend()
	}
	if _, ok := backends[sqlDefaultBackend]; !ok {
		backends[sqlDefaultBackend] = defaultSQLBackend()
	}
	merged[storageBackendsKey] = backends
}

// fillDefaultStores adds default store mappings only for absent keys. An explicit
// null value (key present) is left unchanged, as is a non-object stores section.
func fillDefaultStores(merged map[string]interface{}) {
	stores := asStorageMap(merged[storageStoresKey])
	if stores == nil {
		if _, present := merged[storageStoresKey]; present {
			return
		}
		stores = make(map[string]interface{})
		merged[storageStoresKey] = stores
	}

	for name, mapping := range defaultStores() {
		if _, present := stores[name]; !present {
			stores[name] = mapping
		}
	}
}

func cloneStorageMap(src map[string]interface{}) map[string]interface{} {
	return cloneMap(src)
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = cloneIfMap(v)
	}
	return dst
}

func cloneIfMap(v interface{}) interface{} {
	nested, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	return cloneMap(nested)
}

func asStorageMap(v interface{}) map[string]interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}
