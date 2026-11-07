#!/usr/bin/env bash
#
# offline-install.sh — install storagebench from a prebuilt offline bundle on an
# air-gapped host. No internet, no Go toolchain, no compilation required.
#
# This script ships INSIDE the release bundle (storagebench-offline-*.tar.gz)
# alongside the prebuilt binaries, the controller image tar, and the install
# scripts. Unpack the bundle and run this from its root.
#
# Prerequisites on the target host (install from your SUSE/Ubuntu repo mirror):
#   - Agent  : fio (required); iproute2 (tc), sysstat, smartmontools, nvme-cli,
#              multipath-tools, kmod (optional, enable later phases).
#   - Broker : iproute2 (tc).
#   - Controller: Docker (+ docker compose) if you run the controller here.
#
# Usage examples:
#   # Install the agent (SLES host), pointing at the controller:
#   sudo ./offline-install.sh agent \
#       --controller ws://10.1.1.155:8080/ws/agent --token "SECRET"
#
#   # Install the broker too (enables fault injection):
#   sudo ./offline-install.sh broker
#
#   # Load the controller Docker image (so you can `docker compose up` offline):
#   sudo ./offline-install.sh controller-image
#
# The distro (SLES vs Ubuntu) is auto-detected; override with --distro sles|ubuntu.
#
set -euo pipefail

log()  { printf '\033[1;34m[+]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

BUNDLE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
BIN_DIR="${BUNDLE_DIR}/bin"
SCRIPTS_DIR="${BUNDLE_DIR}/scripts"
IMAGE_TAR="${BUNDLE_DIR}/storagebench-controller-image.tar"

DISTRO=""   # auto-detect unless overridden

# Detect the distro family to pick the right install script.
detect_distro() {
  if [[ -n "$DISTRO" ]]; then return; fi
  if command -v zypper >/dev/null 2>&1; then
    DISTRO="sles"
  elif command -v apt-get >/dev/null 2>&1; then
    DISTRO="ubuntu"
  else
    die "could not detect distro (neither zypper nor apt-get found); pass --distro sles|ubuntu"
  fi
}

script_for() {
  # $1 = agent|broker ; echoes the matching install script path.
  local kind="$1" suffix=""
  [[ "$DISTRO" == "sles" ]] && suffix="-sles"
  echo "${SCRIPTS_DIR}/${kind}/install-${kind}${suffix}.sh"
}

cmd="${1:-}"; shift || true

case "$cmd" in
  agent)
    [[ $EUID -eq 0 ]] || die "run as root (sudo)."
    [[ -f "${BIN_DIR}/storagebench-agent" ]] || die "missing ${BIN_DIR}/storagebench-agent in bundle"
    detect_distro
    s="$(script_for agent)"
    [[ -f "$s" ]] || die "install script not found: $s"
    log "Installing agent (distro=${DISTRO}) from prebuilt binary ..."
    # --binary installs the shipped binary (implies --no-build); --no-deps skips
    # package installation (air-gapped: deps are preinstalled).
    exec bash "$s" --binary "${BIN_DIR}/storagebench-agent" --no-deps "$@"
    ;;
  broker)
    [[ $EUID -eq 0 ]] || die "run as root (sudo)."
    [[ -f "${BIN_DIR}/storagebench-broker" ]] || die "missing ${BIN_DIR}/storagebench-broker in bundle"
    detect_distro
    s="$(script_for broker)"
    [[ -f "$s" ]] || die "install script not found: $s"
    log "Installing broker (distro=${DISTRO}) from prebuilt binary ..."
    exec bash "$s" --binary "${BIN_DIR}/storagebench-broker" "$@"
    ;;
  controller-image)
    [[ -f "$IMAGE_TAR" ]] || die "missing controller image tar: ${IMAGE_TAR}"
    command -v docker >/dev/null 2>&1 || die "docker not found; install Docker to load the controller image"
    log "Loading controller image from ${IMAGE_TAR} ..."
    docker load -i "$IMAGE_TAR"
    log "Image loaded. Tag it / run it, e.g.:"
    echo "    docker compose up -d   (if using the bundled docker-compose.yml)"
    ;;
  --distro)
    die "--distro must follow a subcommand, e.g. 'agent --distro sles'"
    ;;
  ""|-h|--help)
    grep '^#' "$0" | sed 's/^# \{0,1\}//'
    ;;
  *)
    die "unknown subcommand: $cmd (use: agent | broker | controller-image)"
    ;;
esac
