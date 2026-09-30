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
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	backendTypeKVPostgres  = "kv_postgres"
	backendTypeSQLPostgres = "sql_postgres"
	postgresPortMax        = 65535
	postgresDefaultPort    = int32(5432)
	postgresIdentifierMax  = 63
)

// ApplyStorage generates the storage section for config.yaml based on the spec.
// Returns nil if no storage is configured (base config storage is preserved).

// Deprecated: use ApplyStorageWithError to handle named-storage validation errors.
func ApplyStorage(storage *ogxiov1beta1.StateStorageSpec) map[string]interface{} {
	result, _ := ApplyStorageWithError(storage)
	return result
}

// ApplyStorageWithError converts storage settings to OGX config. It returns an
// error for ambiguous default-backend selection, invalid mappings, or a mixture
// of the deprecated and named storage forms.
func ApplyStorageWithError(storage *ogxiov1beta1.StateStorageSpec) (map[string]interface{}, error) {
	if storage == nil {
		return nil, nil
	}

	legacyConfigured := storage.KV != nil || storage.SQL != nil
	namedConfigured := storage.Backends != nil || storage.Stores != nil
	if legacyConfigured && namedConfigured {
		return nil, errors.New("failed to configure storage: spec.storage.backends/stores cannot be combined with deprecated spec.storage.kv/sql fields")
	}
	if namedConfigured {
		return applyNamedStorage(storage)
	}

	return applyLegacyStorage(storage), nil
}

func applyNamedStorage(storage *ogxiov1beta1.StateStorageSpec) (map[string]interface{}, error) {
	if storage.Backends == nil {
		return nil, errors.New("failed to validate storage: spec.storage.backends is required when spec.storage.stores is provided")
	}

	backends := make(map[string]interface{}, len(storage.Backends))
	backendTypes := make(map[string]string, len(storage.Backends))
	backendNames := make([]string, 0, len(storage.Backends))
	for name := range storage.Backends {
		backendNames = append(backendNames, name)
	}
	sort.Strings(backendNames)
	for _, name := range backendNames {
		backend := storage.Backends[name]
		if err := validateNamedBackend(name, backend); err != nil {
			return nil, err
		}
		backends[name] = expandNamedBackend(name, backend)
		backendTypes[name] = backend.Type
	}

	var stores map[string]interface{}
	if storage.Stores == nil {
		kvBackend, err := soleBackendOfFamily(backendTypes, backendTypeKVPostgres, "KV")
		if err != nil {
			return nil, err
		}
		sqlBackend, err := soleBackendOfFamily(backendTypes, backendTypeSQLPostgres, "SQL")
		if err != nil {
			return nil, err
		}
		stores = defaultNamedStores(kvBackend, sqlBackend)
	} else {
		var err error
		stores, err = expandNamedStores(storage.Stores, backendTypes)
		if err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{
		"backends": backends,
		"stores":   stores,
	}, nil
}

func validateNamedBackend(name string, backend ogxiov1beta1.StorageBackendSpec) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("failed to validate storage: spec.storage.backends contains an empty backend name")
	}
	if backend.Type != backendTypeKVPostgres && backend.Type != backendTypeSQLPostgres {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.type must be kv_postgres or sql_postgres", name)
	}
	if err := validateNamedBackendConnection(name, backend); err != nil {
		return err
	}
	if err := validateNamedBackendTypeFields(name, backend); err != nil {
		return err
	}
	return validateNamedBackendNumericFields(name, backend)
}

func validateNamedBackendConnection(name string, backend ogxiov1beta1.StorageBackendSpec) error {
	if backend.Host != "" && strings.TrimSpace(backend.Host) == "" {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.host must not be empty", name)
	}
	if backend.DB != "" && strings.TrimSpace(backend.DB) == "" {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.db must not be empty", name)
	}
	if strings.TrimSpace(backend.User) == "" {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.user is required", name)
	}
	if backend.Port != nil {
		if err := validateOptionalNumericOrEnvValue(name, "port", backend.Port, 1, postgresPortMax); err != nil {
			return err
		}
	}
	return nil
}

func validateNamedBackendTypeFields(name string, backend ogxiov1beta1.StorageBackendSpec) error {
	if backend.Type == backendTypeKVPostgres {
		return validateKVBackendFields(name, backend)
	}
	return validateSQLBackendFields(name, backend)
}

func validateKVBackendFields(name string, backend ogxiov1beta1.StorageBackendSpec) error {
	if backend.TableName != "" && !validKVTableName(backend.TableName) && !isOGXEnvSubstitution(backend.TableName) {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.tableName must be a valid PostgreSQL identifier or OGX environment substitution", name)
	}
	if backend.PoolRecycle != nil {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.poolRecycle is only valid for sql_postgres", name)
	}
	if backend.PoolPrePing != nil || backend.PoolPrePingEnv != "" {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.poolPrePing and poolPrePingEnv are only valid for sql_postgres", name)
	}
	return nil
}

func validateSQLBackendFields(name string, backend ogxiov1beta1.StorageBackendSpec) error {
	if backend.TableName != "" {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.tableName is only valid for kv_postgres", name)
	}
	if backend.CommandTimeout != nil {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.commandTimeout is only valid for kv_postgres", name)
	}
	if backend.SSLMode != "" && !validPostgresSSLMode(backend.SSLMode) && !isOGXEnvSubstitution(backend.SSLMode) {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.sslMode must be a supported SSL mode or an OGX environment substitution", name)
	}
	if backend.PoolPrePingEnv != "" && !isOGXEnvSubstitution(backend.PoolPrePingEnv) {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.poolPrePingEnv must be an OGX environment substitution", name)
	}
	if backend.PoolPrePing != nil && backend.PoolPrePingEnv != "" {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s must set only one of poolPrePing and poolPrePingEnv", name)
	}
	return nil
}

func validateNamedBackendNumericFields(name string, backend ogxiov1beta1.StorageBackendSpec) error {
	if err := validatePositiveFloatOrEnvValue(name, "commandTimeout", backend.CommandTimeout); err != nil {
		return err
	}
	if err := validateOptionalNumericOrEnvValue(name, "poolSize", backend.PoolSize, 1, int64(^uint32(0)>>1)); err != nil {
		return err
	}
	if err := validateOptionalNumericOrEnvValue(name, "maxOverflow", backend.MaxOverflow, 0, int64(^uint32(0)>>1)); err != nil {
		return err
	}
	if backend.Type == backendTypeSQLPostgres {
		if err := validateOptionalNumericOrEnvValue(name, "poolRecycle", backend.PoolRecycle, -1, int64(^uint32(0)>>1)); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalNumericOrEnvValue(name, field string, value *intstr.IntOrString, minValue, maxValue int64) error {
	if value == nil {
		return nil
	}
	var numeric int64
	switch value.Type {
	case intstr.Int:
		numeric = int64(value.IntVal)
	case intstr.String:
		stringValue := strings.TrimSpace(value.StrVal)
		parsed, err := strconv.ParseInt(stringValue, 10, 64)
		if err != nil {
			if isOGXEnvSubstitution(stringValue) {
				return nil
			}
			return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.%s must be an integer or OGX environment substitution", name, field)
		}
		numeric = parsed
	default:
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.%s must be an integer or OGX environment substitution", name, field)
	}
	if numeric < minValue || numeric > maxValue {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.%s must be between %d and %d", name, field, minValue, maxValue)
	}
	return nil
}

func validatePositiveFloatOrEnvValue(name, field string, value *intstr.IntOrString) error {
	if value == nil {
		return nil
	}
	var numeric float64
	switch value.Type {
	case intstr.Int:
		numeric = float64(value.IntVal)
	case intstr.String:
		stringValue := strings.TrimSpace(value.StrVal)
		parsed, err := strconv.ParseFloat(stringValue, 64)
		if err != nil {
			if isOGXEnvSubstitution(stringValue) {
				return nil
			}
			return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.%s must be a positive number or OGX environment substitution", name, field)
		}
		numeric = parsed
	default:
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.%s must be a positive number or OGX environment substitution", name, field)
	}
	if numeric <= 0 {
		return fmt.Errorf("failed to validate storage: spec.storage.backends.%s.%s must be greater than zero", name, field)
	}
	return nil
}

func validKVTableName(name string) bool {
	if len(name) == 0 || len(name) > postgresIdentifierMax {
		return false
	}
	if !isKVTableNameStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isKVTableNamePart(name[i]) {
			return false
		}
	}
	return true
}

func isKVTableNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isKVTableNamePart(c byte) bool {
	return isKVTableNameStart(c) || (c >= '0' && c <= '9')
}

func validPostgresSSLMode(mode string) bool {
	switch mode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

func isOGXEnvSubstitution(value string) bool {
	const prefix = "${env."
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "}") {
		return false
	}
	nameAndDefault := strings.TrimSuffix(strings.TrimPrefix(value, prefix), "}")
	name := strings.SplitN(nameAndDefault, ":=", 2)[0]
	return envVarNameRegex.MatchString(name)
}

func expandNamedBackend(name string, backend ogxiov1beta1.StorageBackendSpec) map[string]interface{} {
	host := backend.Host
	if host == "" {
		host = "localhost"
	}
	database := backend.DB
	if database == "" {
		database = "ogx"
	}
	port := interface{}(postgresDefaultPort)
	if backend.Port != nil {
		port = intOrStringValue(*backend.Port)
	}
	cfg := map[string]interface{}{
		"type": backend.Type,
		"host": host,
		"port": port,
		"db":   database,
		"user": backend.User,
	}
	addNamedBackendConnectionOptions(cfg, name, backend)
	addNamedBackendPoolOptions(cfg, backend)
	return cfg
}

func addNamedBackendConnectionOptions(cfg map[string]interface{}, name string, backend ogxiov1beta1.StorageBackendSpec) {
	if backend.Password != nil {
		cfg["password"] = fmt.Sprintf("${env.%s}", storageBackendPasswordEnvVarName(name))
	}
	if backend.SSLMode != "" {
		cfg["ssl_mode"] = backend.SSLMode
	}
	if backend.CACertPath != "" {
		cfg["ca_cert_path"] = backend.CACertPath
	}
	if backend.TableName != "" {
		cfg["table_name"] = backend.TableName
	}
}

func addNamedBackendPoolOptions(cfg map[string]interface{}, backend ogxiov1beta1.StorageBackendSpec) {
	if backend.PoolSize != nil {
		cfg["pool_size"] = intOrStringValue(*backend.PoolSize)
	}
	if backend.MaxOverflow != nil {
		cfg["max_overflow"] = intOrStringValue(*backend.MaxOverflow)
	}
	if backend.CommandTimeout != nil {
		cfg["command_timeout"] = commandTimeoutValue(*backend.CommandTimeout)
	}
	if backend.PoolRecycle != nil {
		cfg["pool_recycle"] = intOrStringValue(*backend.PoolRecycle)
	}
	if backend.PoolPrePingEnv != "" {
		cfg["pool_pre_ping"] = backend.PoolPrePingEnv
	} else if backend.PoolPrePing != nil {
		cfg["pool_pre_ping"] = *backend.PoolPrePing
	}
}

func intOrStringValue(value intstr.IntOrString) interface{} {
	if value.Type == intstr.String {
		return strings.TrimSpace(value.StrVal)
	}
	return value.IntVal
}

func commandTimeoutValue(value intstr.IntOrString) interface{} {
	if value.Type == intstr.Int {
		return value.IntVal
	}
	stringValue := strings.TrimSpace(value.StrVal)
	if isOGXEnvSubstitution(stringValue) {
		return stringValue
	}
	if numeric, err := strconv.ParseFloat(stringValue, 64); err == nil {
		return numeric
	}
	return stringValue
}

func soleBackendOfFamily(types map[string]string, backendType, family string) (string, error) {
	candidates := make([]string, 0, 1)
	for name, kind := range types {
		if kind == backendType {
			candidates = append(candidates, name)
		}
	}
	sort.Strings(candidates)
	if len(candidates) != 1 {
		return "", fmt.Errorf(
			"failed to generate default stores: expected exactly one %s backend (type %s), found %d; "+
				"set spec.storage.stores explicitly or configure one backend of that type",
			family,
			backendType,
			len(candidates),
		)
	}
	return candidates[0], nil
}

func defaultNamedStores(kvBackend, sqlBackend string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"backend":   kvBackend,
			"namespace": "registry",
		},
		"inference": map[string]interface{}{
			"backend":    sqlBackend,
			"table_name": "inference_store",
		},
		"conversations": map[string]interface{}{
			"backend":    sqlBackend,
			"table_name": "openai_conversations",
		},
		"prompts": map[string]interface{}{
			"backend":    sqlBackend,
			"table_name": "prompts",
		},
		"connectors": map[string]interface{}{
			"backend":    sqlBackend,
			"table_name": "connectors",
		},
	}
}

func expandNamedStores(specStores *ogxiov1beta1.StorageStoresSpec, backendTypes map[string]string) (map[string]interface{}, error) {
	// StackConfig models stores as ServerStoresConfig, whose Pydantic defaults
	// populate metadata/inference/conversations/prompts/connectors when keys are
	// absent. Null explicitly disables those defaults and preserves the API's
	// complete-object semantics, including stores: {}.
	if specStores == nil {
		return nil, errors.New("failed to convert storage: spec.storage.stores is required for explicit-store conversion")
	}

	stores := map[string]interface{}{
		"metadata":      nil,
		"inference":     nil,
		"conversations": nil,
		"responses":     nil,
		"prompts":       nil,
		"connectors":    nil,
		"vector_stores": nil,
	}
	if err := expandMetadataStore(stores, specStores.Metadata, backendTypes); err != nil {
		return nil, err
	}
	if err := expandInferenceStore(stores, specStores.Inference, backendTypes); err != nil {
		return nil, err
	}
	if err := expandSQLStoreMappings(stores, specStores, backendTypes); err != nil {
		return nil, err
	}
	if err := expandResponsesStore(stores, specStores.Responses, backendTypes); err != nil {
		return nil, err
	}
	return stores, nil
}

func expandMetadataStore(
	stores map[string]interface{},
	store *ogxiov1beta1.KVStoreMappingSpec,
	backendTypes map[string]string,
) error {
	if store == nil {
		return nil
	}
	if err := validateStoreBackend("metadata", store.Backend, "kv", backendTypes); err != nil {
		return err
	}
	if strings.TrimSpace(store.Namespace) == "" {
		return errors.New("failed to validate storage: spec.storage.stores.metadata.namespace is required")
	}
	stores["metadata"] = map[string]interface{}{"backend": store.Backend, "namespace": store.Namespace}
	return nil
}

func expandInferenceStore(
	stores map[string]interface{},
	store *ogxiov1beta1.InferenceStoreMappingSpec,
	backendTypes map[string]string,
) error {
	if store == nil {
		return nil
	}
	if err := validateStoreBackend("inference", store.Backend, "sql", backendTypes); err != nil {
		return err
	}
	if strings.TrimSpace(store.TableName) == "" {
		return errors.New("failed to validate storage: spec.storage.stores.inference.tableName is required")
	}
	cfg := map[string]interface{}{"backend": store.Backend, "table_name": store.TableName}
	if store.MaxWriteQueueSize != nil {
		cfg["max_write_queue_size"] = *store.MaxWriteQueueSize
	}
	if store.NumWriters != nil {
		cfg["num_writers"] = *store.NumWriters
	}
	if store.Enabled != nil {
		cfg["enabled"] = *store.Enabled
	}
	stores["inference"] = cfg
	return nil
}

func expandSQLStoreMappings(
	stores map[string]interface{},
	specStores *ogxiov1beta1.StorageStoresSpec,
	backendTypes map[string]string,
) error {
	for _, entry := range []struct {
		name  string
		store *ogxiov1beta1.SQLStoreMappingSpec
	}{
		{name: "conversations", store: specStores.Conversations},
		{name: "prompts", store: specStores.Prompts},
		{name: "connectors", store: specStores.Connectors},
		{name: "vector_stores", store: specStores.VectorStores},
	} {
		if entry.store == nil {
			continue
		}
		if err := validateStoreBackend(entry.name, entry.store.Backend, "sql", backendTypes); err != nil {
			return err
		}
		if strings.TrimSpace(entry.store.TableName) == "" {
			return fmt.Errorf("failed to validate storage: spec.storage.stores.%s.tableName is required", entry.name)
		}
		stores[entry.name] = map[string]interface{}{
			"backend":    entry.store.Backend,
			"table_name": entry.store.TableName,
		}
	}
	return nil
}

func expandResponsesStore(
	stores map[string]interface{},
	store *ogxiov1beta1.ResponsesStoreMappingSpec,
	backendTypes map[string]string,
) error {
	if store == nil {
		return nil
	}
	if err := validateStoreBackend("responses", store.Backend, "sql", backendTypes); err != nil {
		return err
	}
	cfg := map[string]interface{}{"backend": store.Backend}
	if store.TableName != "" {
		cfg["table_name"] = store.TableName
	}
	if store.MaxWriteQueueSize != nil {
		cfg["max_write_queue_size"] = *store.MaxWriteQueueSize
	}
	if store.NumWriters != nil {
		cfg["num_writers"] = *store.NumWriters
	}
	stores["responses"] = cfg
	return nil
}

func validateStoreBackend(storeName, backendName, family string, backendTypes map[string]string) error {
	if strings.TrimSpace(backendName) == "" {
		return fmt.Errorf("failed to validate storage: spec.storage.stores.%s.backend is required", storeName)
	}
	backendType, exists := backendTypes[backendName]
	if !exists {
		return fmt.Errorf("failed to validate storage: spec.storage.stores.%s.backend references unknown backend %q", storeName, backendName)
	}
	if !strings.HasPrefix(backendType, family+"_") {
		return fmt.Errorf("failed to validate storage: spec.storage.stores.%s.backend %q must reference a %s backend", storeName, backendName, family)
	}
	return nil
}

// applyLegacyStorage keeps the existing SQLite, Redis, and SQL DSN conversion
// unchanged during the storage API compatibility period.
func applyLegacyStorage(storage *ogxiov1beta1.StateStorageSpec) map[string]interface{} {
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
			"type":              backendTypeSQLPostgres,
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
