#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
requested_field=${1:-version}
stable_version=$(tr -d '[:space:]' < "$repo_root/VERSION")

case "$stable_version" in
  ''|*[!0-9.]*|*.*.*.*)
    echo "VERSION 必须是三段稳定语义版本" >&2
    exit 1
    ;;
esac

old_ifs=$IFS
IFS=.
set -- $stable_version
IFS=$old_ifs
if [ "$#" -ne 3 ] || [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
  echo "VERSION 必须是三段稳定语义版本" >&2
  exit 1
fi

commit=$(git -C "$repo_root" rev-parse HEAD)
short_commit=$(printf '%s' "$commit" | cut -c1-12)
source_date_epoch=$(git -C "$repo_root" show -s --format=%ct HEAD)
if build_date=$(date -u -d "@$source_date_epoch" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null); then
  dev_date=$(date -u -d "@$source_date_epoch" '+%Y%m%d')
else
  build_date=$(date -u -r "$source_date_epoch" '+%Y-%m-%dT%H:%M:%SZ')
  dev_date=$(date -u -r "$source_date_epoch" '+%Y%m%d')
fi

event_name=${GITHUB_EVENT_NAME:-local}
ref_type=${GITHUB_REF_TYPE:-branch}
ref_name=${GITHUB_REF_NAME:-}

if [ "$event_name" != "workflow_dispatch" ] && [ "$ref_type" = "tag" ]; then
  expected_tag="v$stable_version"
  if [ "$ref_name" != "$expected_tag" ]; then
    echo "tag $ref_name 与 VERSION 不一致，期望 $expected_tag" >&2
    exit 1
  fi
  tag_commit=$(git -C "$repo_root" rev-list -n 1 "$ref_name")
  if [ "$tag_commit" != "$commit" ]; then
    echo "tag $ref_name 未指向当前检出提交" >&2
    exit 1
  fi
  version=$stable_version
  release_tag=$ref_name
  prerelease=false
  release_name="GoDoIt $version"
  channel=stable
else
  version="$stable_version-dev.$dev_date.$short_commit"
  release_tag=dev-latest
  prerelease=true
  release_name="GoDoIt Development Build $version"
  channel=development
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    printf 'version=%s\n' "$version"
    printf 'stable_version=%s\n' "$stable_version"
    printf 'commit=%s\n' "$commit"
    printf 'build_date=%s\n' "$build_date"
    printf 'source_date_epoch=%s\n' "$source_date_epoch"
    printf 'tag=%s\n' "$release_tag"
    printf 'prerelease=%s\n' "$prerelease"
    printf 'release_name=%s\n' "$release_name"
    printf 'channel=%s\n' "$channel"
  } >> "$GITHUB_OUTPUT"
fi

field=$requested_field
case "$field" in
  version) printf '%s\n' "$version" ;;
  stable_version) printf '%s\n' "$stable_version" ;;
  commit) printf '%s\n' "$commit" ;;
  build_date) printf '%s\n' "$build_date" ;;
  source_date_epoch) printf '%s\n' "$source_date_epoch" ;;
  tag) printf '%s\n' "$release_tag" ;;
  prerelease) printf '%s\n' "$prerelease" ;;
  release_name) printf '%s\n' "$release_name" ;;
  channel) printf '%s\n' "$channel" ;;
  *)
    echo "未知版本字段 $field" >&2
    exit 2
    ;;
esac
