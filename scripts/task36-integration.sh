#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mode=${1:?usage: task36-integration.sh local|published|released [root-tag]}
task36_dir=$(mktemp -d "${TMPDIR:-/tmp}/toolsy-task36.XXXXXX")
trap 'rm -rf -- "$task36_dir"' EXIT
cp -R "$root/examples/host_dispatch_integration/." "$task36_dir/"
case "$mode" in
 local)
  : "${PROMPTY_DIR:?set PROMPTY_DIR to its checkout}"
  : "${FLOWY_DIR:?set FLOWY_DIR to its checkout}"
  : "${GUARDY_DIR:?set GUARDY_DIR to its checkout}"
  PROMPTY_DIR=$(CDPATH= cd -- "$PROMPTY_DIR" && pwd)
  FLOWY_DIR=$(CDPATH= cd -- "$FLOWY_DIR" && pwd)
  GUARDY_DIR=$(CDPATH= cd -- "$GUARDY_DIR" && pwd)
  test "$(GOWORK=off go -C "$PROMPTY_DIR" list -m -f '{{.Path}}')" = github.com/skosovsky/prompty
  test "$(GOWORK=off go -C "$FLOWY_DIR" list -m -f '{{.Path}}')" = github.com/skosovsky/flowy
  test "$(GOWORK=off go -C "$GUARDY_DIR" list -m -f '{{.Path}}')" = github.com/skosovsky/guardy
  (cd "$task36_dir" && GOWORK=off go work init "$root" "$task36_dir" "$PROMPTY_DIR" "$FLOWY_DIR" "$GUARDY_DIR")
  (cd "$task36_dir" && GOWORK="$task36_dir/go.work" go test -mod=readonly -race -count=1 -v ./...)
  ;;
 published)
  # Before publication the candidate recipe is consumer code, tested against
  # published dependencies without a replace or workspace. No core code is copied.
  mkdir "$task36_dir/recipe"
  cp "$root/examples/host_dispatch/recipe/dispatch.go" "$root/examples/host_dispatch/recipe/profile.go" "$task36_dir/recipe/"
  for task36_source in "$task36_dir"/*.go; do
   sed 's@github.com/skosovsky/toolsy/examples/host_dispatch/recipe@github.com/skosovsky/toolsy/examples/host_dispatch_integration/recipe@g' "$task36_source" > "$task36_source.tmp"
   mv "$task36_source.tmp" "$task36_source"
  done
  (cd "$task36_dir" && GOWORK=off go mod tidy && GOWORK=off go test -race -count=1 -v ./...)
  ;;
 released)
  task36_tag=${2:?released mode needs the published root tag}
  (cd "$task36_dir" && GOWORK=off go get "github.com/skosovsky/toolsy@$task36_tag" && GOWORK=off go mod tidy && GOWORK=off go test -race -count=1 -v ./...)
  ;;
 *) echo "unsupported mode: $mode" >&2; exit 1;;
esac
