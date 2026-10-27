#!/usr/bin/env bash
set -euo pipefail

expected_format='<package>: <lowercase description>'

declare -A packages=()

log_tmp=

cleanup_log() {
  if [[ -n ${log_tmp} ]]; then
    rm -f -- "$log_tmp"
    log_tmp=
  fi
}

trap cleanup_log EXIT

packages_at() {
  local commit="$1" path dir files
  unset packages
  declare -gA packages=()
  files=$(git ls-tree -r --name-only "$commit" -- cmd internal) || exit 1
  while IFS= read -r path; do
    [[ $path == *.go ]] || continue
    dir=${path%/*}
    case /$dir/ in
      */testdata/*) continue ;;
    esac
    packages[$dir]=1
  done <<< "$files"
  if ((${#packages[@]} == 0)); then
    printf 'no Go packages under cmd or internal at %s\n' "$commit" >&2
    exit 1
  fi
}

subject_ok() {
  local subject="$1" prefix desc part rest
  [[ $subject == *": "* ]] || return 1
  prefix=${subject%%": "*}
  desc=${subject#*": "}
  [[ $desc =~ ^[a-z] ]] || return 1
  if [[ $prefix == all || $prefix == meta ]]; then
    return 0
  fi
  rest=$prefix
  local -A seen=()
  while true; do
    if [[ $rest == *", "* ]]; then
      part=${rest%%", "*}
      rest=${rest#*", "}
    else
      part=$rest
      rest=
    fi
    [[ -n $part && -n ${packages[$part]+x} && -z ${seen[$part]+x} ]] || return 1
    seen[$part]=1
    [[ -n $rest ]] || return 0
  done
}

check_range() {
  local base_hash head_hash hash subject status
  base_hash=$(git rev-parse --verify --end-of-options "${1}^{commit}") || exit 1
  head_hash=$(git rev-parse --verify --end-of-options "${2}^{commit}") || exit 1
  log_tmp=$(mktemp)
  git log -z --max-parents=1 --format='%H%x00%s' \
    "${base_hash}..${head_hash}" >"$log_tmp" || exit 1
  status=0
  while IFS= read -r -d '' hash && IFS= read -r -d '' subject; do
    packages_at "$hash"
    if ! subject_ok "$subject"; then
      printf '%s: subject must match "%s" (a package path such as internal/tsutil, a comma-separated list such as internal/ui, internal/gutil, or all, or meta); got "%s"\n' \
        "$hash" "$expected_format" "$subject" >&2
      status=1
    fi
  done < "$log_tmp"
  cleanup_log
  return "$status"
}

main() {
  if (($# != 2)); then
    printf 'usage: bash .github/check-commit-subjects.sh <base-rev> <head-rev>\n' >&2
    exit 1
  fi
  check_range "$1" "$2"
}

main "$@"
