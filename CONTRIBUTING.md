# Contributing to OGX K8s Operator

Thank you for your interest in contributing to the OGX K8s Operator! This document provides guidelines and instructions for contributing to this project.

## Development Setup

### Prerequisites

- Go (version specified in go.mod)
- Make
- pre-commit

### Pre-commit Hooks

This project uses pre-commit hooks to ensure code quality and consistency. The pre-commit hooks are automatically run on every commit and are also checked in our CI pipeline.

#### Setting up pre-commit

1. Install pre-commit by reference to the [pre-commit docs](https://pre-commit.com/#install)

2. Install the pre-commit hooks (optional):
   ```bash
   pre-commit install
   ```

#### Running Pre-commit Manually

You can run pre-commit hooks manually on all files:

```bash
pre-commit run --all-files
```

Or on specific files:

```bash
pre-commit run --files file1 file2
```

### CI Checks

The pre-commit hooks are also run in our CI pipeline on every pull request and push to the main branch. The CI will fail if:

1. Any pre-commit hooks fail
2. There are uncommitted changes after running pre-commit
3. There are new files that haven't been committed

To avoid CI failures, always run pre-commit locally before pushing your changes:

```bash
pre-commit run --all-files
git add .
git commit -m "Your commit message"
```

## Pull Request Process

1. Ensure your code passes all pre-commit checks locally
2. Create a pull request against the main branch
3. Ensure all CI checks pass
4. Wait for review and address any feedback

## Code Style

Please follow the project's code style guidelines. The pre-commit hooks will help enforce many of these automatically.

All error messages in the codebase must follow a consistent format to improve readability and maintainability. The pre-commit hook `check-go-error-messages` enforces these rules automatically.

### Rules for Error Messages

1. All **wrapped** error messages must start with "failed to"
2. Error messages should be descriptive and actionable

## Testing

### Running Tests

The project uses `make test` to run the unit tests. By default, this runs all tests except end-to-end tests with code coverage.

```bash
# Run all tests (default behavior)
make test

# Run tests for a specific package
make test TEST_PKGS=./pkg/deploy

# Run a specific test function
make test TEST_PKGS=./pkg/deploy TEST_FLAGS="-v -run TestRenderManifest"

# Run tests with verbose output
make test TEST_FLAGS="-v -coverprofile cover.out"

# Run tests for multiple packages
make test TEST_PKGS="./pkg/deploy ./controllers"
```

### Test Configuration Variables

The `make test` target supports the following variables for customization:

- `TEST_PKGS` - Space-separated list of packages to test (default: all packages except e2e)
- `TEST_FLAGS` - Additional flags to pass to `go test` (default: `-coverprofile cover.out`)

### Test-Driven Development (TDD)

For rapid development cycles, you can use focused test runs:

```bash
# Example TDD workflow for working on the deploy package
make test TEST_PKGS=./pkg/deploy TEST_FLAGS="-v -run TestRenderManifest"
```

## Anti-Fragile Testing Principles

### Test Structure
- **DAMP and DRY together**: Apply DAMP (Descriptive And Meaningful Phrases) to test scenarios, and DRY to implementation details. Keep test intent explicit while extracting common setup logic.
- **Use AAA pattern**: Arrange, Act, Assert with clear comments separating each phase.

### Test Data
- **Integration tests**: Use production constants to verify the controller applies defaults correctly.
- **E2E tests**: Use test-owned constants to focus on user workflows, not implementation details.
- **Make test intent obvious**: Use descriptive names and explicit values that show what each test is verifying.

### Test Isolation
- **No shared state**: Each test should be independent with unique namespaces and fresh instances.
- **Use builders for variations**: Create test instances with clear intent using the builder pattern.
- **Async operations**: Use `require.Eventually` with appropriate timeouts for Kubernetes operations.

### Example
```go
// Good: Descriptive test names that explain behavior
{
    name: "No storage configuration - should use emptyDir",
    buildInstance: func(namespace string) *ogxiov1beta1.OGXServer {
        return NewServerBuilder().
            WithStorage(nil).
            Build()
    },
},

// Bad: Vague test name that doesn't explain expected behavior
{
    name: "test nil storage",
    // ...
}
```

Focus: Tests should survive refactoring and clearly communicate what behavior they're verifying.

## Questions?

If you have any questions about contributing, please open an issue in the repository.
