#!/usr/bin/env bash
# Copyright (c) Meta Platforms, Inc. and affiliates.
# All rights reserved.
#
# This source code is licensed under the terms described in the LICENSE file in
# the root directory of this source tree.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
kustomize=$(realpath "${KUSTOMIZE:-bin/kustomize}")
yq=$(realpath "${YQ:-bin/yq}")
hook_entry=$("$yq" -r '.repos[] | select(.repo == "local") | .hooks[] | select(.id == "build-installer") | .entry' .pre-commit-config.yaml)
read -r -a hook_command <<< "$hook_entry"

sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
cp Makefile "$sandbox/"
cp -R config release "$sandbox/"
image_base=quay.io/ogx-ai/ogx-k8s-operator

# Exercise the real hook and renderer; Go code generation is checked separately.
export MAKEFLAGS='--no-print-directory -o manifests -o generate -o kustomize'
# GNU Make does not forward -o to recursive invocations through MAKEFLAGS.
export MAKE="make $MAKEFLAGS"
export KUSTOMIZE="$kustomize"

assert_image() {
  local expected=$1 file actual
  for file in operator.yaml operator-openshift.yaml; do
    actual=$("$yq" -r 'select(.kind == "Deployment") | .spec.template.spec.containers[] | select(.name == "manager") | .image' "$sandbox/release/$file")
    if [[ "$actual" != "$expected" ]]; then
      echo "Failed to preserve controller image in $file: expected $expected, got $actual" >&2
      exit 1
    fi
  done
}

for tag in latest v0.17.0; do
  (cd "$sandbox/config/manager" && "$kustomize" edit set image "controller=$image_base:$tag")
  cp "$sandbox/config/manager/kustomization.yaml" "$sandbox/expected-kustomization.yaml"
  "$kustomize" build "$sandbox/config/overlays/cert-manager" > "$sandbox/release/operator.yaml"
  "$kustomize" build "$sandbox/config/overlays/openshift" > "$sandbox/release/operator-openshift.yaml"
  cp "$sandbox/release/operator.yaml" "$sandbox/expected-operator.yaml"
  cp "$sandbox/release/operator-openshift.yaml" "$sandbox/expected-operator-openshift.yaml"

  (cd "$sandbox" && "${hook_command[@]}")
  cmp "$sandbox/expected-kustomization.yaml" "$sandbox/config/manager/kustomization.yaml"
  cmp "$sandbox/expected-operator.yaml" "$sandbox/release/operator.yaml"
  cmp "$sandbox/expected-operator-openshift.yaml" "$sandbox/release/operator-openshift.yaml"
  assert_image "$image_base:$tag"
  echo "PASS: hook preserves $tag and leaves current installers unchanged"

  printf '# stale installer\n' > "$sandbox/release/operator.yaml"
  printf '# stale installer\n' > "$sandbox/release/operator-openshift.yaml"
  (cd "$sandbox" && "${hook_command[@]}")
  cmp "$sandbox/expected-operator.yaml" "$sandbox/release/operator.yaml"
  cmp "$sandbox/expected-operator-openshift.yaml" "$sandbox/release/operator-openshift.yaml"
  assert_image "$image_base:$tag"
  echo "PASS: hook regenerates both stale installers for $tag"
done

(cd "$sandbox" && make build-installer "IMG=$image_base:v0.17.1")
actual_tag=$("$yq" -r '.images[] | select(.name == "controller") | .newTag' "$sandbox/config/manager/kustomization.yaml")
[[ "$actual_tag" == v0.17.1 ]]
assert_image "$image_base:v0.17.1"
echo 'PASS: explicit build-installer IMG override updates the configured image and both installers'
