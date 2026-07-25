#!/bin/sh
set -e

# Docker runs as root; the workspace is owned by the runner user.
# Mark it safe so git operations work inside the container.
git config --global --add safe.directory "${GITHUB_WORKSPACE:-/github/workspace}"

command="${INPUT_COMMAND:?INPUT_COMMAND is required}"

case "${command}" in
  snapshot)
    exec radm snapshot \
      --owner "${INPUT_OWNER:?INPUT_OWNER is required for snapshot}" \
      --settings-path "${INPUT_SETTINGS_PATH:-.github}"
    ;;
  create-pr)
    exec radm create-pr \
      --settings-path "${INPUT_SETTINGS_PATH:-.github}" \
      --branch "${INPUT_BRANCH:-settings-sync/snapshot}" \
      --base "${INPUT_BASE:-main}"
    ;;
  cleanup-runs)
    # owner is optional: when omitted the tool auto-detects the current
    # repository from GITHUB_REPOSITORY, which GitHub injects into the run.
    set --
    [ -n "${INPUT_OWNER:-}" ] && set -- "$@" --owner "${INPUT_OWNER}"
    [ -n "${INPUT_CLEANUP_REPO:-}" ] && set -- "$@" --repo "${INPUT_CLEANUP_REPO}"
    [ -n "${INPUT_CLEANUP_DAYS:-}" ] && set -- "$@" --days "${INPUT_CLEANUP_DAYS}"
    [ -n "${INPUT_CLEANUP_KEEP:-}" ] && set -- "$@" --keep "${INPUT_CLEANUP_KEEP}"
    [ "${INPUT_CLEANUP_DRY_RUN:-false}" = "true" ] && set -- "$@" --dry-run
    exec radm cleanup-runs "$@"
    ;;
  dependabot-sweep)
    # radm splits a comma-separated --owner value itself, so the owner list
    # passes through as one argument and no word splitting happens here.
    set -- --owner "${INPUT_SWEEP_OWNERS:?INPUT_SWEEP_OWNERS is required for dependabot-sweep}"
    [ -n "${INPUT_SWEEP_EXCLUDES:-}" ] && set -- "$@" --exclude "${INPUT_SWEEP_EXCLUDES}"
    set -- "$@" --merge-method "${INPUT_SWEEP_MERGE_METHOD:-squash}"
    set -- "$@" --rate-floor "${INPUT_SWEEP_RATE_FLOOR:-200}"
    [ "${INPUT_SWEEP_ALLOW_MAJOR:-false}" = "true" ] && set -- "$@" --allow-major
    [ "${INPUT_SWEEP_DRY_RUN:-false}" = "true" ] && set -- "$@" --dry-run
    exec radm dependabot-sweep "$@"
    ;;
  *)
    echo "Unknown command: ${command}" >&2
    exit 1
    ;;
esac
