#!/usr/bin/env bash
#
# install-broker.sh — deploy the storagebench privilege-broker on a host.
#
# The privilege-broker is the ONLY storagebench component that runs as root. It
# applies and reverts network faults (via tc) on behalf of the unprivileged
# agent, over a local Unix socket, enforcing a strict allowlist, argv-only
# execution, a watchdog auto-revert, and an audit log.
#
# What it does:
#   1. Builds the static broker binary from this repository (installs Go if the
#      system Go is missing or too old). CGO is disabled -> fully static.
#   2. Ensures the 'storagebench' group exists (the agent user joins it so it can
#      reach the broker socket; the socket is group 0660).
#   3. Creates the runtime socket dir and the audit log dir.
#   4. Installs and starts a systemd service running as root.
#   5. Points the agent config at the broker socket and restarts the agent.
#
# Usage (run as root, from inside the cloned repo):
#
#   sudo ./deploy/broker/install-broker.sh
#
# Optional flags:
#   --prefix DIR     Install dir for the binary (default: /usr/local/bin)
#   --from-release   Download nightly binary from GitHub Releases (no compile)
#   --from-image     Extract the binary from ghcr.io/.../storagebench-broker
#   --build          Compile from this repo (default on Ubuntu)
#   --binary PATH    Install this prebuilt broker binary instead of building from
#                    source (offline/air-gapped). Implies --no-build.
#   --no-build       Use an existing binary from --prefix instead of building
#   --no-agent-wire  Do not modify the agent config / restart the agent
#
set -euo pipefail

# ---- defaults -------------------------------------------------------------
PREFIX="/usr/local/bin"
DO_BUILD="true"
FETCH_SOURCE="${STORAGEBENCH_FETCH:-}"
WIRE_AGENT="true"
# Optional: install a prebuilt broker binary instead of building from source
# (offline/air-gapped deploys). Implies --no-build.
BINARY_SRC="${STORAGEBENCH_BROKER_BINARY:-}"

SERVICE_USER="storagebench"            # group the agent runs as / joins
BINARY_NAME="storagebench-broker"
SERVICE_NAME="storagebench-broker"
RUN_DIR="/run/storagebench"
SOCKET_PATH="${RUN_DIR}/broker.sock"
LOG_DIR="/var/log/storagebench"
AUDIT_PATH="${LOG_DIR}/broker-audit.log"
CONFIG_DIR="/etc/storagebench"
CONFIG_FILE="${CONFIG_DIR}/agent.json"
AGENT_SERVICE="storagebench-agent"
AGENT_BINARY="storagebench-agent"
GO_MIN_MINOR=22
GO_VERSION="1.22.9"

# ---- helpers --------------------------------------------------------------
log()  { printf '\033[1;34m[+]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)"
# shellcheck source=../lib/fetch-prebuilt.sh
source "${REPO_ROOT}/deploy/lib/fetch-prebuilt.sh"

# ---- argument parsing -----------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)        PREFIX="$2"; shift 2 ;;
    --from-release)  FETCH_SOURCE="release"; DO_BUILD="false"; shift ;;
    --from-image)    FETCH_SOURCE="image"; DO_BUILD="false"; shift ;;
    --build)         FETCH_SOURCE=""; DO_BUILD="true"; shift ;;
    --binary)        BINARY_SRC="$2"; FETCH_SOURCE=""; DO_BUILD="false"; shift 2 ;;
    --no-build)      FETCH_SOURCE=""; DO_BUILD="false"; shift ;;
    --no-agent-wire) WIRE_AGENT="false"; shift ;;
    -h|--help)       grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (use --help)" ;;
  esac
done

# ---- preflight ------------------------------------------------------------
[[ $EUID -eq 0 ]] || die "please run as root (sudo)."
command -v apt-get >/dev/null 2>&1 || warn "apt-get not found; ensure iproute2 (tc) is installed."
# tc is required for network faults.
if ! command -v tc >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    log "Installing iproute2 (provides tc) ..."
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iproute2 || die "could not install iproute2"
  else
    die "tc (iproute2) not found and cannot auto-install on this distro."
  fi
fi
log "tc: $(command -v tc)"

# ---- Go toolchain + build -------------------------------------------------
go_ok() {
  command -v go >/dev/null 2>&1 || return 1
  local ver minor
  ver="$(go env GOVERSION 2>/dev/null | sed 's/^go//')" || return 1
  minor="$(printf '%s' "$ver" | cut -d. -f2)"
  [[ "${minor:-0}" -ge "$GO_MIN_MINOR" ]]
}

install_go() {
  local arch tgz url
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported CPU architecture for Go install: $(uname -m)" ;;
  esac
  tgz="go${GO_VERSION}.linux-${arch}.tar.gz"
  url="https://go.dev/dl/${tgz}"
  log "Installing Go ${GO_VERSION} (${arch}) to /usr/local/go ..."
  local tmp; tmp="$(mktemp -d)"
  command -v curl >/dev/null 2>&1 || { apt-get install -y --no-install-recommends curl tar ca-certificates; }
  curl -fsSL "$url" -o "${tmp}/${tgz}" || die "failed to download Go from ${url}"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "${tmp}/${tgz}"
  rm -rf "$tmp"
  export PATH="/usr/local/go/bin:${PATH}"
}

build_broker() {
  [[ -f "${REPO_ROOT}/go.mod" ]] || die "cannot find go.mod at ${REPO_ROOT}; run from inside the cloned repo."
  if go_ok; then
    log "Using system Go: $(go version)"
  else
    install_go
    go_ok || die "Go installation did not yield a usable toolchain."
  fi
  log "Building ${BINARY_NAME} (static, CGO disabled) ..."
  ( cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux \
      go build -trimpath -ldflags "-s -w" \
      -o "/tmp/${BINARY_NAME}.build" ./cmd/broker ) || die "go build failed"
  install -m 0755 "/tmp/${BINARY_NAME}.build" "${PREFIX}/${BINARY_NAME}"
  rm -f "/tmp/${BINARY_NAME}.build"
  log "Installed binary: ${PREFIX}/${BINARY_NAME}"
}

# ---- group, dirs ----------------------------------------------------------
ensure_group_and_dirs() {
  if ! getent group "$SERVICE_USER" >/dev/null 2>&1; then
    log "Creating group '${SERVICE_USER}' ..."
    groupadd --system "$SERVICE_USER"
  fi
  # If the agent user exists, make sure it is in the group so it can use the socket.
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    usermod -aG "$SERVICE_USER" "$SERVICE_USER" 2>/dev/null || true
  fi
  # Audit log dir (root-owned).
  install -d -m 0750 -o root -g "$SERVICE_USER" "$LOG_DIR"
  # The runtime socket dir is created by systemd via RuntimeDirectory, but create
  # it now too so a manual run works. Group-accessible so the agent can connect.
  install -d -m 0750 -o root -g "$SERVICE_USER" "$RUN_DIR"
}

# ---- systemd service ------------------------------------------------------
write_service() {
  local unit="/etc/systemd/system/${SERVICE_NAME}.service"
  log "Installing systemd unit ${unit} ..."
  cat > "$unit" <<UNIT
[Unit]
Description=storagebench privilege-broker (network fault injection, root)
After=network.target

[Service]
Type=simple
# The broker must be root to run tc. This is the only privileged component.
User=root
Group=root
# systemd creates RuntimeDirectory owned by the service user/group (root:root
# here), mode 0750 -> the agent (group storagebench) would lack the directory
# traverse (x) bit and cannot reach the socket. Fix it: chgrp the directory to the
# storagebench group so group members get r-x. The socket itself stays protected
# at 0660 root:storagebench, so only the broker group can connect.
RuntimeDirectory=storagebench
RuntimeDirectoryMode=0750
ExecStartPre=-/bin/sh -c 'chgrp ${SERVICE_USER} ${RUN_DIR} && chmod 0750 ${RUN_DIR}'
ExecStart=${PREFIX}/${BINARY_NAME} --socket ${SOCKET_PATH} --group ${SERVICE_USER} --audit ${AUDIT_PATH}
Restart=always
RestartSec=3
# Hardening: the broker needs CAP_NET_ADMIN for tc and must write the socket and
# audit log. Everything else is locked down.
NoNewPrivileges=false
ProtectSystem=full
ReadWritePaths=${LOG_DIR} ${RUN_DIR}
ProtectHome=true
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT

  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
  systemctl restart "$SERVICE_NAME"
}

# ---- wire the agent -------------------------------------------------------
wire_agent() {
  [[ -f "$CONFIG_FILE" ]] || { warn "agent config ${CONFIG_FILE} not found; skipping agent wiring."; return; }

  # Warn loudly if the installed agent binary predates the fault-injection
  # feature: wiring the config alone would not help, and a later agent re-deploy
  # with an old installer could overwrite this key again.
  local agent_bin="${PREFIX}/${AGENT_BINARY}"
  if [[ -x "$agent_bin" ]]; then
    if ! strings "$agent_bin" 2>/dev/null | grep -q "management interface detected"; then
      warn "The installed agent binary (${agent_bin}) does NOT support fault"
      warn "injection. Rebuild/redeploy the agent from the current repo:"
      warn "  sudo ./deploy/agent/install-agent.sh --controller ... --token ... --no-deps"
    fi
  fi

  if grep -q '"brokerSocket"' "$CONFIG_FILE"; then
    log "Agent config already references brokerSocket; leaving as-is."
  else
    log "Adding brokerSocket to ${CONFIG_FILE} ..."
    # Insert the brokerSocket key after the opening brace. Uses a temp file so we
    # never leave a half-written config.
    local tmp; tmp="$(mktemp)"
    awk -v sock="$SOCKET_PATH" '
      NR==1 && $0 ~ /\{/ { print; print "  \"brokerSocket\": \"" sock "\","; next }
      { print }
    ' "$CONFIG_FILE" > "$tmp"
    if command -v python3 >/dev/null 2>&1; then
      python3 -c "import json,sys;json.load(open('$tmp'))" \
        || die "failed to insert brokerSocket (invalid JSON); left ${CONFIG_FILE} untouched."
    fi
    install -m 0640 -o root -g "$SERVICE_USER" "$tmp" "$CONFIG_FILE"
    rm -f "$tmp"
  fi

  # Make sure the agent user is in the broker group so it can reach the socket,
  # then restart the agent so the new group membership and config take effect.
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    usermod -aG "$SERVICE_USER" "$SERVICE_USER" 2>/dev/null || true
  fi
  if systemctl list-unit-files | grep -q "^${AGENT_SERVICE}.service"; then
    log "Restarting ${AGENT_SERVICE} to pick up the broker socket ..."
    systemctl restart "$AGENT_SERVICE" || warn "could not restart ${AGENT_SERVICE}"
  else
    warn "${AGENT_SERVICE}.service not found; restart the agent manually so it picks up brokerSocket."
  fi
}

# ---- run ------------------------------------------------------------------
log "storagebench privilege-broker deploy"
log "  repo root : ${REPO_ROOT}"
log "  socket    : ${SOCKET_PATH}"
log "  audit log : ${AUDIT_PATH}"

install_fetched_or_built broker "${PREFIX}/${BINARY_NAME}"

ensure_group_and_dirs
write_service
[[ "$WIRE_AGENT" == "true" ]] && wire_agent || warn "skipping agent wiring (--no-agent-wire)"

log "Done. Service status:"
systemctl --no-pager --full status "$SERVICE_NAME" || true
cat <<TIP

Next steps:
  * Follow logs:   journalctl -u ${SERVICE_NAME} -f
  * Audit log:     sudo tail -f ${AUDIT_PATH}
  * The agent now advertises fault-injection capability once it reconnects.

Security notes:
  * The broker is the only root component. It executes only 'tc' (allowlisted),
    always as an argv (never a shell).
  * The interface carrying the controller connection is auto-protected by the
    agent, so a fault can never cut the host off from the controller.
  * Every applied fault has a watchdog that auto-reverts it if it is not
    explicitly removed in time.
TIP
