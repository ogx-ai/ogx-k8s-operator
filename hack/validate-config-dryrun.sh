#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="$SCRIPT_DIR/validate-config-dryrun.env"

if [[ ! -f "$ENV_FILE" ]]; then
    echo "Error: $ENV_FILE not found"
    echo "Copy the example and edit it:"
    echo "  cp $ENV_FILE.example $ENV_FILE"
    exit 2
fi

# shellcheck source=/dev/null
source "$ENV_FILE"

: "${CR_PATH:?CR_PATH must be set in $ENV_FILE}"
: "${OGX_GIT_REF:?OGX_GIT_REF must be set in $ENV_FILE}"
: "${OGX_GIT_REPO:?OGX_GIT_REPO must be set in $ENV_FILE}"

if ! command -v uvx &>/dev/null; then
    echo "Error: uvx not found. Install uv: https://docs.astral.sh/uv/"
    exit 2
fi

CONFIGGEN="$REPO_ROOT/bin/configgen"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Building configgen..."
(cd "$REPO_ROOT" && go build -o "$CONFIGGEN" ./cmd/configgen/)

CONFIGGEN_ARGS=(-output-config)
if [[ -n "${BASE_PATH:-}" ]]; then
    CONFIGGEN_ARGS+=(-base "$REPO_ROOT/$BASE_PATH")
fi
CONFIGGEN_ARGS+=("$REPO_ROOT/$CR_PATH")

CONFIG_FILE="$TMPDIR/config.yaml"

echo "Generating config from $CR_PATH..."
"$CONFIGGEN" "${CONFIGGEN_ARGS[@]}" > "$CONFIG_FILE"

echo "--- generated config ---"
cat "$CONFIG_FILE"
echo "------------------------"

echo ""
echo "Running ogx dry-run (${OGX_GIT_REPO}@${OGX_GIT_REF})..."
uvx --from "ogx @ git+${OGX_GIT_REPO}@${OGX_GIT_REF}" ogx run "$CONFIG_FILE" --dry-run
