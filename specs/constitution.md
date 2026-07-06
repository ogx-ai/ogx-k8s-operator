# Project Constitution: llama-stack-k8s-operator

**Purpose**: Define project-wide principles, patterns, and standards that guide all specifications and implementations.

**Last Updated**: 2026-02-19
**Version**: 1.5

---

## Constitution Status

This constitution documents **existing patterns** found in the codebase as of 2025-11-12. Rules are categorized as:

- **Current Practice**: Patterns consistently used in existing code (marked with ✅)
- **Aspirational**: Patterns we want to adopt going forward (marked with 🎯)
- **MUST**: Non-negotiable requirements for all new code
- **SHOULD**: Strong recommendations, exceptions allowed with justification

**Adoption Approach**: This is a living document. Apply these patterns when writing new code or significantly refactoring existing code. No need for wholesale rewrites.

---

## Quick Reference: Top 11 Critical Rules

Quick lookup for the most important rules during day-to-day development:

1. **✅ Reconciliation is Idempotent** (§1.2)
   Reconcile functions must produce same result when called multiple times with same input

2. **✅ Separate Reconciliation from Status** (§1.2)
   Pattern: `reconcileResources(ctx, instance)` separate from `updateStatus(ctx, instance, err)`

3. **✅ Wrap Errors with Context** (§4.1)
   Always use `fmt.Errorf("failed to X: %w", err)` to preserve error chain

4. **✅ Use Kubebuilder Validation** (§2.1)
   Field validation at CRD level prevents invalid resources: `// +kubebuilder:validation:Required`

5. **✅ Status Has Phase + Conditions** (§3)
   Status MUST include phase enum and Kubernetes standard conditions (`metav1.Condition`)

6. **✅ Logger in Context** (§5.1)
   Store logger with request values in context, retrieve in sub-functions

7. **✅ Table-Driven Tests** (§6.1)
   Use `tests := []struct{name, input, expected}` pattern for multiple test cases

8. **✅ Builder Pattern for Tests** (§6.4)
   Test instances created with `NewDistributionBuilder().WithX().Build()`

9. **✅ Owner References for Cleanup** (§1.3)
   All owned resources must have owner references for automatic garbage collection

10. **✅ Namespace-Scoped Resources** (§1.1)
    Operator must NOT require cluster-admin or use cluster-scoped resources

11. **✅ FIPS Compliance** (§14) — All crypto, TLS, and RNG usage MUST use FIPS-validated implementations.

**Legend**: ✅ = Current practice | 🎯 = Aspirational | See section § for details

---

## 1. Kubernetes Operator Principles

### 1.1 Resource Scope
- **MUST**: All resources MUST be namespace-scoped (no cluster-wide permissions)
- **MUST**: Operator MUST NOT require cluster-admin privileges
- **RATIONALE**: Enables multi-tenant deployments and follows least-privilege principle

### 1.2 Reconciliation
- **MUST**: Reconciliation MUST be idempotent
- **MUST**: Reconciler MUST handle partial failures gracefully
- **MUST**: Status updates MUST occur even when reconciliation fails
- **PATTERN**: Separate reconciliation logic (`reconcileResources`) from status updates (`updateStatus`)
- **RATIONALE**: Ensures observable state even during failures

### 1.3 Resource Ownership
- **MUST**: Owned resources MUST use owner references for garbage collection
- **MUST**: Resources owned by the operator MUST have consistent labeling (see Naming Conventions)
- **RATIONALE**: Enables automatic cleanup on CR deletion

---

## 2. API Design (CRD)

### 2.1 Validation

- **MUST**: Use kubebuilder validation tags for field-level validation
  ```go
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
  // +kubebuilder:validation:Enum=Value1;Value2;Value3
  ```

- **MUST**: Use XValidation for complex cross-field validation
  ```go
  // +kubebuilder:validation:XValidation:rule="!(has(self.name) && has(self.image))",message="Only one of name or image can be specified"
  ```

- **SHOULD**: Provide clear, actionable validation error messages
- **RATIONALE**: Fail fast at admission time rather than reconciliation time

### 2.2 Optional Fields

- **MUST**: Mark optional fields with `+optional` tag
- **MUST**: Use pointers for optional structs to distinguish between "not set" and "set to zero value"
  ```go
  Storage *StorageSpec `json:"storage,omitempty"` // +optional
  ```

- **RATIONALE**: Clear distinction between unset and zero value

### 2.3 Defaults

- **MUST**: Define constants for default values
  ```go
  const (
      DefaultContainerName = "llama-stack"
      DefaultServerPort int32 = 8321
  )
  ```

- **SHOULD**: Use kubebuilder default tags where appropriate
  ```go
  // +kubebuilder:default:=1
  Replicas int32 `json:"replicas,omitempty"`
  ```

- **RATIONALE**: Single source of truth for defaults, testable

### 2.4 Status Subresource

- **MUST**: Use status subresource for all CRDs
  ```go
  //+kubebuilder:subresource:status
  ```

- **MUST**: Status MUST include Phase field (enum of valid phases)
- **MUST**: Status MUST include Conditions (Kubernetes standard metav1.Condition)
- **SHOULD**: Include version information in status for observability

### 2.5 Printer Columns

- **MUST**: Define useful printer columns for `kubectl get` output
  ```go
  //+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
  //+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
  ```

- **RATIONALE**: Better user experience with kubectl

### 2.6 Short Names

- **MUST**: Define short names for CRDs
  ```go
  //+kubebuilder:resource:shortName=llsd
  ```

- **RATIONALE**: Improves CLI usability

---

## 3. Status Reporting

### 3.1 Status Phases

- **MUST**: Define phase as enum type
  ```go
  // +kubebuilder:validation:Enum=Pending;Initializing;Ready;Failed;Terminating
  type DistributionPhase string
  ```

- **MUST**: Define constants for each phase
  ```go
  const (
      LlamaStackDistributionPhasePending DistributionPhase = "Pending"
      LlamaStackDistributionPhaseReady   DistributionPhase = "Ready"
      // ...
  )
  ```

### 3.2 Conditions

- **MUST**: Use Kubernetes standard `metav1.Condition` type
- **MUST**: Define constants for condition types, reasons, and messages
  ```go
  const (
      // Condition types
      ConditionTypeDeploymentReady = "DeploymentReady"

      // Condition reasons
      ReasonDeploymentReady = "DeploymentReady"
      ReasonDeploymentFailed = "DeploymentFailed"

      // Condition messages
      MessageDeploymentReady = "Deployment is ready"
      MessageDeploymentFailed = "Deployment failed"
  )
  ```

- **MUST**: Provide helper functions for setting conditions
  ```go
  func SetDeploymentReadyCondition(status *Status, ready bool, message string)
  ```

- **MUST**: Include timestamps using `metav1.NewTime(metav1.Now().UTC())`
- **SHOULD**: Provide helper functions for checking condition state
  ```go
  func IsConditionTrue(status *Status, conditionType string) bool
  ```

### 3.3 Condition Update Pattern

- **MUST**: Use generic `SetCondition` function that updates existing or appends new
  ```go
  func SetCondition(status *Status, condition metav1.Condition) {
      // Find and update existing, or append new
  }
  ```

- **RATIONALE**: Avoids duplicate conditions, maintains condition list integrity

---

## 4. Error Handling

### 4.1 Error Wrapping

- **MUST**: Wrap errors with context using `%w` verb
  ```go
  return fmt.Errorf("failed to reconcile storage: %w", err)
  ```

- **MUST**: Include resource identifiers in error messages
  ```go
  return fmt.Errorf("failed to fetch ConfigMap %s/%s: %w", namespace, name, err)
  ```

- **RATIONALE**: Preserves error chain for debugging, provides context

### 4.2 Error Messages

- **MUST**: Error messages MUST be descriptive and include:
  - Operation that failed
  - Resource identifier (name, namespace)
  - Context about why it matters

- **SHOULD**: Start error messages with lowercase (Go convention)
  ```go
  // Good
  return fmt.Errorf("failed to create deployment: %w", err)

  // Bad
  return fmt.Errorf("Failed to create deployment: %w", err)
  ```

### 4.3 User-Facing Errors

- **MUST**: User-facing error messages (in status conditions) MUST be actionable
- **MUST**: Include what went wrong and how to fix it
- **EXAMPLE**:
  ```
  "Failed to find referenced ConfigMap user-config/default.
   Ensure the ConfigMap exists in the specified namespace."
  ```

### 4.4 Not Found Errors

- **MUST**: Check for `IsNotFound` errors when fetching optional resources
  ```go
  if err != nil {
      if k8serrors.IsNotFound(err) {
          // Handle not found case
          return fmt.Errorf("failed to find referenced ConfigMap %s/%s", ns, name)
      }
      return fmt.Errorf("failed to fetch ConfigMap %s/%s: %w", ns, name, err)
  }
  ```

---

## 5. Logging

### 5.1 Logger Pattern

- **MUST**: Store logger in context with request-specific values
  ```go
  logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
  ctx = logr.NewContext(ctx, logger)
  ```

- **MUST**: Retrieve logger from context in sub-functions
  ```go
  logger := log.FromContext(ctx)
  ```

- **RATIONALE**: Consistent logging across reconciliation, no need to pass logger

### 5.2 Log Levels

- **Info**: Normal operations, state transitions
- **Error**: Errors that need attention (with error object)
  ```go
  logger.Error(err, "failed to reconcile deployment")
  ```

- **V(1)**: Debug-level information
- **MUST NOT**: Use `fmt.Printf` or `log.Printf` - always use structured logging

### 5.3 Log Messages

- **MUST**: Use structured logging with key-value pairs
  ```go
  logger.Info("reconciling deployment",
      "deployment", deploymentName,
      "replicas", replicas)
  ```

---

## 6. Testing

### 6.1 Test Organization

- **MUST**: Use table-driven tests for multiple test cases
  ```go
  tests := []struct {
      name     string
      input    *Input
      expected *Expected
  }{
      {name: "test case 1", ...},
      {name: "test case 2", ...},
  }

  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
          // test logic
      })
  }
  ```

### 6.2 Test Helpers

- **MUST**: Use `t.Helper()` in helper functions
  ```go
  func AssertDeploymentExists(t *testing.T, ...) {
      t.Helper()
      // assertion logic
  }
  ```

- **MUST**: Use `require` package for assertions (not `assert`)
- **RATIONALE**: `require` stops test on first failure, `assert` continues

### 6.3 Test Cleanup

- **MUST**: Use `t.Cleanup()` for resource cleanup
  ```go
  t.Cleanup(func() {
      if err := k8sClient.Delete(ctx, resource); err != nil {
          t.Logf("cleanup failed: %v", err)
      }
  })
  ```

### 6.4 Builder Pattern

- **MUST**: Use builder pattern for creating test instances
  ```go
  instance := NewDistributionBuilder().
      WithName("test").
      WithNamespace(namespace).
      WithStorage(DefaultTestStorage()).
      Build()
  ```

- **MUST**: Return deep copy from `Build()` method
  ```go
  func (b *DistributionBuilder) Build() *LlamaStackDistribution {
      return b.instance.DeepCopy()
  }
  ```

- **RATIONALE**: Immutability, prevents test contamination

### 6.5 Async Assertions

- **MUST**: Use `require.Eventually()` for async checks
  ```go
  require.Eventually(t, func() bool {
      err := k8sClient.Get(ctx, key, resource)
      return err == nil
  }, timeout, interval, "resource should exist")
  ```

- **MUST**: Define test timeout and interval as constants
  ```go
  const (
      testTimeout  = 5 * time.Second
      testInterval = 100 * time.Millisecond
  )
  ```

### 6.6 Test Naming

- **MUST**: Use descriptive test names that explain what is being tested
  ```go
  "No storage configuration - should use emptyDir"
  "Storage with custom values"
  ```

- **RATIONALE**: Test name documents expected behavior

---

## 7. Code Organization

### 7.1 File Structure

- **MUST**: Separate concerns into different files:
  - `*_types.go` - API types and CRD definitions
  - `*_controller.go` - Main reconciliation logic
  - `status.go` - Status condition helpers
  - `resource_helper.go` - Resource construction helpers
  - `*_test.go` - Tests
  - `testing_support_test.go` - Test helpers and builders

### 7.2 Function Organization

- **SHOULD**: Keep functions focused on single responsibility
- **SHOULD**: Extract complex logic into helper functions
- **SHOULD**: Place helper functions close to where they're used

### 7.3 Helper Function Patterns

- **PATTERN**: Provide both receiver methods and standalone helpers
  - Receiver methods when reconciler context is needed
  - Standalone helpers for use in other packages or tests

  ```go
  // Receiver method (has access to r.Client)
  func (r *Reconciler) hasUserConfigMap(instance *CR) bool

  // Standalone helper (used in watches, predicates)
  func hasValidUserConfig(instance *CR) bool
  ```

### 7.4 Constants

- **MUST**: Define constants for:
  - Default values
  - Resource names
  - Condition types, reasons, messages
  - Well-known labels and annotations

- **MUST**: Group related constants together
  ```go
  const (
      // Condition types
      ConditionTypeX = "X"
      ConditionTypeY = "Y"
  )
  ```

---

## 8. Naming Conventions

### 8.1 Resource Naming

- **PATTERN**: Owned resources use CR name as prefix
  ```
  {cr-name}-deployment
  {cr-name}-service
  {cr-name}-pvc
  ```

### 8.2 Labels

- **MUST**: Apply consistent labels to all owned resources
  ```go
  DefaultLabelKey   = "app"
  DefaultLabelValue = "llama-stack"
  ```

- **SHOULD**: Include additional labels for:
  - CR name
  - Component type
  - Managed-by operator

### 8.3 Variable Naming

- **MUST**: Use descriptive variable names
  ```go
  // Good
  configMapNamespace := r.getUserConfigMapNamespace(instance)

  // Bad
  ns := r.getUserConfigMapNamespace(instance)
  ```

- **EXCEPTION**: Loop variables can be short (`i`, `j`, `k`)

---

## 9. Dependencies and Imports

### 9.1 Import Organization

- **MUST**: Organize imports in standard Go order:
  1. Standard library
  2. External dependencies
  3. Internal packages

- **TOOL**: Use `gci` linter to enforce (configured in `.golangci.yml`)

### 9.2 Import Aliases

- **MUST**: Use standard aliases for common packages:
  ```go
  corev1 "k8s.io/api/core/v1"
  appsv1 "k8s.io/api/apps/v1"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  k8serrors "k8s.io/apimachinery/pkg/api/errors"
  llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
  ```

---

## 10. Documentation

### 10.1 Code Comments

- **MUST**: Export public functions, types, and constants have godoc comments
  ```go
  // LlamaStackDistributionReconciler reconciles a LlamaStack object.
  type LlamaStackDistributionReconciler struct { ... }
  ```

- **SHOULD**: Explain non-obvious logic with inline comments
- **MUST**: Document why, not what (code shows what)

### 10.2 Example Files

- **MUST**: Provide sample YAML files in `config/samples/`
- **MUST**: Include inline comments explaining configuration options
  ```yaml
  # Uncomment the storage section to use persistent storage
  # storage: {}  # Will use default size of 10Gi
  ```

### 10.3 API Documentation

- **MUST**: Use kubebuilder markers for field documentation
  ```go
  // Size is the size of the persistent volume claim
  // +kubebuilder:validation:Required
  Size *resource.Quantity `json:"size"`
  ```

### 10.4 Specification Document Formatting

- **MUST**: Use proper markdown bullet lists for all enumerated items
  ```markdown
  # Good - Proper markdown list
  - ✅ **Feature 1**: Description
  - ✅ **Feature 2**: Description
  - ❌ **Anti-pattern**: Description

  # Bad - Items without list markers
  ✅ **Feature 1**: Description
  ✅ **Feature 2**: Description
  ❌ **Anti-pattern**: Description
  ```

- **MUST**: Keep icons/emojis (✅, ❌, 🎯, etc.) but format as list items
- **RATIONALE**: Proper markdown lists render correctly on GitHub with consistent indentation and spacing
- **APPLIES TO**: All specification documents (spec.md, plan.md, tasks.md, pr-strategy.md, etc.)

---

## 11. Feature Flags

### 11.1 Feature Flag Pattern

- **MUST**: Use boolean fields in reconciler for feature flags
  ```go
  type Reconciler struct {
      client.Client
      Scheme *runtime.Scheme
      EnableNetworkPolicy bool  // Feature flag
  }
  ```

- **MUST**: Check feature flags before executing feature-specific code
  ```go
  if r.EnableNetworkPolicy {
      // NetworkPolicy logic
  }
  ```

- **RATIONALE**: Allows gradual rollout and easy disablement

---

## 12. API Versioning

### 12.1 Version Stability

- **CURRENT**: `v1alpha1` indicates API is not stable
- **FUTURE**: Promote to `v1beta1` when API stabilizes
- **FUTURE**: Promote to `v1` when API is production-ready

### 12.2 Breaking Changes

- **MUST**: Breaking changes in alpha versions are acceptable
- **MUST**: Beta versions should minimize breaking changes
- **MUST**: v1 versions MUST maintain backwards compatibility

### 12.3 Deprecation

- **MUST**: Deprecated fields MUST include deprecation notice in comments
- **MUST**: Maintain deprecated fields for at least one minor version
- **SHOULD**: Log warnings when deprecated fields are used

---

## 13. Git Commit Guidelines

### 13.1 Commit Messages

- **MUST**: Use conventional commit format when applicable (feat:, fix:, docs:, etc.)
- **SHOULD**: Include context and reasoning in commit body for non-trivial changes
- **SHOULD**: Reference related issues/PRs when applicable

### 13.2 AI-Assisted Commits

- **MUST NOT**: Include `Co-Authored-By: Claude <noreply@anthropic.com>` or similar AI co-author attributions
- **RATIONALE**: AI co-authorship causes Contributor License Agreement (CLA) check failures
- **MUST**: Use `Assisted-by:` trailer for AI-assisted commits
- **FORMAT**: Add trailer after commit message body, before `Signed-off-by:`

**AI Attribution Format**:
```
Assisted-by: Claude Code
```

**Example commit message**:
```bash
git commit -s -m "feat: add new feature

This feature implements...

Assisted-by: Claude Code
Signed-off-by: Your Name <your.email@example.com>"
```

**Rationale**: The `Assisted-by:` trailer provides a consistent format for acknowledging AI assistance while maintaining CLA compliance and avoiding co-authorship issues.

### 13.3 Commit Sign-Off

- **MUST**: All commits MUST be signed off using `git commit --signoff` (or `-s`)
- **RATIONALE**: Sign-off indicates agreement with Developer Certificate of Origin (DCO)
- **FORMAT**: Adds `Signed-off-by: Your Name <your.email@example.com>` to commit message
- **ENFORCEMENT**: Pre-commit hooks or CI checks should verify sign-off is present

**Example**:
```bash
git commit -s -m "feat: add new feature"
# Results in:
# feat: add new feature
#
# Signed-off-by: John Doe <john.doe@example.com>
```

### 13.4 Commit Hygiene

- **SHOULD**: Make atomic commits (one logical change per commit)
- **SHOULD**: Ensure each commit passes tests and builds successfully
- **MUST**: Commits MUST pass all pre-commit hooks before being accepted

---

## 14. Strict FIPS Compliance Guidelines

This project MUST adhere strictly to "Designed for FIPS" standards at all times. The AI agent MUST enforce the following constraints:
- **RATIONALE**: Ensures the operator meets federal security compliance requirements and can be deployed in regulated environments.

### 14.1 Cryptographic Algorithms
- **MUST**: NEVER use MD5, SHA-1, DES, or any non-FIPS-validated cryptographic algorithms for any purpose (even for non-security functions like file hashing). ALWAYS use FIPS 140-2/140-3 approved algorithms (e.g., SHA-256, SHA-3, AES).

### 14.2 Network Traffic
- **MUST**: NEVER implement or allow unencrypted network connections. ALWAYS enforce TLS (version 1.2 or higher) for all inbound and outbound traffic.

### 14.3 Ciphers & Security Contexts
- **MUST**: ONLY implement FIPS-approved TLS cipher suites and security contexts. Reject any configuration that permits fallback to non-compliant ciphers.

### 14.4 UUIDs & Randomization
- **MUST**: NEVER use standard, non-secure random number generators (RNGs). Any UUID generation or randomization MUST rely exclusively on FIPS-validated Random Number Generators (e.g., leveraging the OS's FIPS-compliant crypto module).

---

## Enforcement

This constitution is enforced through **automated tooling** and **code review practices**:

### Automated Enforcement

The project uses **pre-commit hooks** (`.pre-commit-config.yaml`) that run on every commit:

#### Standard Checks
- **Trailing whitespace** - Removed automatically
- **End-of-file fixer** - Ensures files end with newline
- **Large files** - Prevents commits > 1000KB
- **YAML validation** - Validates all YAML files
- **Private key detection** - Prevents accidental credential commits
- **Line ending consistency** - Forces LF (not CRLF)

#### Go-Specific Checks
- **`make lint`** - Runs golangci-lint with configured linters
  - **gci**: Import organization (stdlib → external → internal)
  - **gocyclo**: Cyclomatic complexity (max 30)
  - **errcheck**: Unchecked errors
  - **goconst**: Repeated strings that should be constants
  - **revive**: Style and best practices
  - **gocritic**: Bug detection and performance
  - Full configuration in `.golangci.yml`

- **`make generate manifests`** - Regenerates CRD manifests from code
  - Ensures kubebuilder markers are up-to-date
  - Validates CRD schema

- **`make api-docs`** - Regenerates API documentation
  - Keeps documentation in sync with code

- **`./hack/check_go_errors.py`** - Custom error message validation
  - Enforces lowercase error messages (Go convention)
  - Ensures error wrapping patterns

#### Security Checks
- **GitHub Actions pinning** - Ensures workflow actions use SHA hashes (not tags)
- **Prevents direct commits to main** - Forces pull requests

### Code Review Checklist

When reviewing code, verify:

- [ ] Constitution compliance for changed code (see Quick Reference)
- [ ] Kubebuilder validation tags for new CRD fields
- [ ] Status conditions updated appropriately
- [ ] Errors wrapped with `%w` and include context
- [ ] Tests use builder pattern and table-driven structure
- [ ] Owner references set on created resources
- [ ] Logging uses context logger with structured fields
- [ ] No `fmt.Printf` for logging
- [ ] No non-FIPS cryptographic algorithms (MD5, SHA-1, DES) used
- [ ] All network connections use TLS 1.2+

### Exceptions and Deviations

If you need to deviate from a MUST rule:

1. **Document why** in code comments
2. **Get approval** in PR review
3. **Create issue** if pattern needs updating
4. **Update constitution** if exception becomes common pattern

Example:
```go
// Deviation from constitution §4.1: Not wrapping error here because
// the error is immediately returned to user and stack trace not needed.
// See issue #123 for discussion.
return errors.New("invalid configuration")
```

---

## Adoption Guidelines

### For New Features

1. Read this constitution before starting implementation
2. Follow existing patterns for consistency
3. Add new patterns to constitution if they're project-wide
4. Update constitution when patterns change

### For Existing Code

- **Gradual Adoption**: Apply constitution patterns when touching code
- **No Big Rewrite**: Don't refactor everything at once
- **Document Exceptions**: If constitution doesn't apply, document why

### For Code Reviews

- Constitution compliance is a review criterion
- Suggest improvements based on constitution
- Challenge patterns that diverge without good reason

---

## References

- **Kubebuilder Book**: https://book.kubebuilder.io/
- **Kubernetes API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **Controller-runtime**: https://pkg.go.dev/sigs.k8s.io/controller-runtime
