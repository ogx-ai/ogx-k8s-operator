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

// ApplyStorage generates the storage section for config.yaml based on the spec.
// Returns nil if no storage is configured (base config storage is preserved).
func ApplyStorage(storage *ogxiov1beta1.StateStorageSpec) map[string]interface{} {
	if storage == nil {
		return nil
	}

	backends := make(map[string]interface{})
	stores := defaultStores()

	if storage.KV != nil {
		backends["kv_default"] = expandKVBackend(storage.KV)
	} else {
		backends["kv_default"] = defaultKVBackend()
	}

	if storage.SQL != nil {
		backends["sql_default"] = expandSQLBackend(storage.SQL)
	} else {
		backends["sql_default"] = defaultSQLBackend()
	}

	return map[string]interface{}{
		"backends": backends,
		"stores":   stores,
	}
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
			"backend":   "kv_default",
			"namespace": "registry",
		},
		"inference": map[string]interface{}{
			"backend":    "sql_default",
			"table_name": "inference_store",
		},
		"conversations": map[string]interface{}{
			"backend":    "sql_default",
			"table_name": "openai_conversations",
		},
		"prompts": map[string]interface{}{
			"backend":   "kv_default",
			"namespace": "prompts",
		},
	}
}
