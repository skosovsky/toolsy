#!/bin/bash
set -euo pipefail
# The asynchronous bootstrap build owns a process group on supported hosts.
set -m

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE
if [ -n "${GIT_CONFIG_PARAMETERS:-}" ]; then
    echo "release does not support GIT_CONFIG_PARAMETERS; use Git config" >&2
    exit 1
fi
if [ "${GIT_CONFIG_COUNT:-0}" != 0 ]; then
    echo "bootstrap does not support counted Git config; use Git config or the native CLI" >&2
    exit 1
fi

script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
runner_dir=$(mktemp -d "${TMPDIR:-/tmp}/toolsy-release-runner.XXXXXX")
child_pid=
child_phase=
cleanup() {
    if [ -n "$child_pid" ]; then
        if [ "$child_phase" = build ]; then
            kill -TERM -- "-$child_pid" 2>/dev/null || true
            for attempt in {1..20}; do
                kill -0 "$child_pid" 2>/dev/null || break
                sleep 0.1
            done
            kill -KILL -- "-$child_pid" 2>/dev/null || true
        else
            kill -TERM "$child_pid" 2>/dev/null || true
        fi
        wait "$child_pid" 2>/dev/null || true
    fi
    rm -rf -- "$runner_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

# Bootstrap only committed code; untracked Go files must not enter the runner.
if [ -n "$(GIT_OPTIONAL_LOCKS=0 git -C "$script_root" -c core.fsmonitor=false status --porcelain --untracked-files=no)" ]; then
    echo "commit or stash tracked changes before release" >&2
    exit 1
fi
# Archive transformations must be rejected before any runner code is built.
GIT_OPTIONAL_LOCKS=0 git -C "$script_root" ls-tree -r -z --name-only HEAD |
    git -C "$script_root" check-attr --cached -z --stdin filter working-tree-encoding export-ignore export-subst ident crlf text eol |
    while IFS= read -r -d '' attribute_path; do
        IFS= read -r -d '' attribute_name
        IFS= read -r -d '' attribute_value
        if [ "$attribute_value" != unspecified ] && [ "$attribute_value" != unset ]; then
            echo "release does not support transforming Git attribute $attribute_name=$attribute_value on $attribute_path" >&2
            exit 1
        fi
    done
mkdir "$runner_dir/source"
# Drain the complete archive before extraction: some tar implementations stop at
# the end marker before consuming padding, giving git SIGPIPE under pipefail.
GIT_OPTIONAL_LOCKS=0 git -C "$script_root" -c core.autocrlf=input -c core.eol=lf archive --output="$runner_dir/source.tar" HEAD
tar -xf "$runner_dir/source.tar" -C "$runner_dir/source"
rm -- "$runner_dir/source.tar"
GOWORK=off go build -C "$runner_dir/source" -mod=readonly -buildvcs=false -o "$runner_dir/toolsy-release" ./cmd/toolsy-release &
child_pid=$!
child_phase=build
wait "$child_pid"
child_pid=
# Keep the CLI in the terminal's foreground process group for its prompt.
set +m
"$runner_dir/toolsy-release" "$@" <&0 &
child_pid=$!
child_phase=release
wait "$child_pid"
child_pid=
