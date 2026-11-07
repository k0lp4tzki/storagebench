#!/usr/bin/env bash
#
# install-agent.sh — deploy the storagebench agent on an Ubuntu host.
#
# What it does:
#   1. Installs dependencies: fio (required) plus useful tools for later phases
#      (smartctl, nvme, iostat, multipath, device-mapper, tc).
#   2. Builds the static agent binary from this repository (installs Go if the
#      system Go is missing or too old). CGO is disabled, so the binary is fully
#      static and portable.
#   3. Creates a dedicated system user, config file, and spool directory.
#   4. Installs and starts a systemd service that reconnects automatically.
#
# Usage (run as root, from anywhere; the script finds the repo it lives in):
#
#   sudo ./deploy/agent/install-agent.sh \
#       --controller ws://CONTROLLER_HOST:8080/ws/agent \
#       --token "YOUR_SHARED_TOKEN"
#
# Optional flags:
#   --agent-id NAME        Stable agent id (default: system hostname)
#   --broker-socket PATH   Enable fault injection via a local privilege-broker
#                          socket. If omitted, an existing brokerSocket already in
#                          the config is PRESERVED (re-running never disables it).
#   --from-release         Download nightly binary from GitHub Releases (no compile)
#   --from-image           Extract the binary from ghcr.io/.../storagebench-agent
#   --build                Compile from this repo (default on Ubuntu)
#   --binary PATH          Install this prebuilt agent binary instead of building
#                          from source (offline/air-gapped). Implies --no-build.
#   --insecure             Skip TLS verification (self-signed wss:// only)
#   --prefix DIR           Install dir for the binary (default: /usr/local/bin)
#   --no-deps              Do not install apt packages (assume already present)
#   --no-build             Use an existing binary from --prefix instead of building
#
# Environment variables are also honored:
#   STORAGEBENCH_CONTROLLER_URL, STORAGEBENCH_AGENT_TOKEN, STORAGEBENCH_AGENT_ID,
#   STORAGEBENCH_BROKER_SOCKET
#
set -euo pipefail

# ---- defaults -------------------------------------------------------------
CONTROLLER_URL="${STORAGEBENCH_CONTROLLER_URL:-}"
TOKEN="${STORAGEBENCH_AGENT_TOKEN:-}"
AGENT_ID="${STORAGEBENCH_AGENT_ID:-}"
# Optional: path to the local privilege-broker socket (enables fault injection).
# If left empty, an existing brokerSocket already in the config is PRESERVED, so
# re-running this installer never silently disables fault injection.
BROKER_SOCKET="${STORAGEBENCH_BROKER_SOCKET:-}"
INSECURE="false"
PREFIX="/usr/local/bin"
INSTALL_DEPS="true"
DO_BUILD="true"
FETCH_SOURCE="${STORAGEBENCH_FETCH:-}"
# Optional: install a prebuilt agent binary instead of building from source
# (offline/air-gapped deploys). Implies --no-build.
BINARY_SRC="${STORAGEBENCH_AGENT_BINARY:-}"

SERVICE_USER="storagebench"
CONFIG_DIR="/etc/storagebench"
CONFIG_FILE="${CONFIG_DIR}/agent.json"
ENV_FILE="${CONFIG_DIR}/agent.env"
SPOOL_DIR="/var/lib/storagebench/agent-spool"
BINARY_NAME="storagebench-agent"
SERVICE_NAME="storagebench-agent"
GO_MIN_MINOR=22          # require Go 1.22+
GO_VERSION="1.22.9"      # version installed if system Go is missing/old

# ---- helpers --------------------------------------------------------------
log()  { printf '\033[1;34m[+]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

# Resolve the repository root (two levels up from this script: deploy/agent/..).
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)"
# shellcheck source=../lib/fetch-prebuilt.sh
source "${REPO_ROOT}/deploy/lib/fetch-prebuilt.sh"

# ---- argument parsing -----------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --controller)    CONTROLLER_URL="$2"; shift 2 ;;
    --token)         TOKEN="$2"; shift 2 ;;
    --agent-id)      AGENT_ID="$2"; shift 2 ;;
    --broker-socket) BROKER_SOCKET="$2"; shift 2 ;;
    --from-release)  FETCH_SOURCE="release"; DO_BUILD="false"; shift ;;
    --from-image)    FETCH_SOURCE="image"; DO_BUILD="false"; shift ;;
    --build)         FETCH_SOURCE=""; DO_BUILD="true"; shift ;;
    --binary)        BINARY_SRC="$2"; FETCH_SOURCE=""; DO_BUILD="false"; shift 2 ;;
    --insecure)      INSECURE="true"; shift ;;
    --prefix)     PREFIX="$2"; shift 2 ;;
    --no-deps)    INSTALL_DEPS="false"; shift ;;
    --no-build)   FETCH_SOURCE=""; DO_BUILD="false"; shift ;;
    -h|--help)    grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (use --help)" ;;
  esac
done

# ---- preflight ------------------------------------------------------------
[[ $EUID -eq 0 ]] || die "please run as root (sudo)."
[[ -n "$CONTROLLER_URL" ]] || die "missing --controller ws://HOST:PORT/ws/agent"
[[ -n "$TOKEN" ]] || die "missing --token (shared controller token)"
case "$CONTROLLER_URL" in
  ws://*|wss://*) : ;;
  *) die "controller URL must start with ws:// or wss://, got: $CONTROLLER_URL" ;;
esac
command -v apt-get >/dev/null 2>&1 || die "this script targets Ubuntu/Debian (apt-get not found)."

if [[ "$AGENT_ID" == "" ]]; then
  AGENT_ID="$(hostname -s 2>/dev/null || hostname)"
fi

# ---- 1. dependencies ------------------------------------------------------
install_deps() {
  log "Installing dependencies via apt (fio is required; others enable later phases)..."
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  # fio: the benchmark engine (required).
  # sysstat: iostat. smartmontools: smartctl. nvme-cli: nvme.
  # multipath-tools: multipath/dmsetup. iproute2: tc. kmod: modprobe (dm-* modules).
  apt-get install -y --no-install-recommends \
    fio sysstat smartmontools nvme-cli multipath-tools iproute2 kmod ca-certificates \
    || die "apt-get install failed"
}

ensure_fio() {
  command -v fio >/dev/null 2>&1 || die "fio is not installed and could not be installed. The agent cannot run benchmarks without fio."
  log "fio: $(fio --version 2>/dev/null || echo unknown)"
}

# ---- 2. Go toolchain + build ---------------------------------------------
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
  curl -fsSL "$url" -o "${tmp}/${tgz}" || die "failed to download Go from ${url}"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "${tmp}/${tgz}"
  rm -rf "$tmp"
  export PATH="/usr/local/go/bin:${PATH}"
}

build_agent() {
  [[ -f "${REPO_ROOT}/go.mod" ]] || die "cannot find go.mod at ${REPO_ROOT}; run this script from inside the cloned repo."
  if go_ok; then
    log "Using system Go: $(go version)"
  else
    command -v curl >/dev/null 2>&1 || { apt-get install -y --no-install-recommends curl tar; }
    install_go
    go_ok || die "Go installation did not yield a usable toolchain."
    log "Using Go: $(go version)"
  fi

  log "Building ${BINARY_NAME} (static, CGO disabled) ..."
  ( cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux \
      go build -trimpath -ldflags "-s -w" \
      -o "/tmp/${BINARY_NAME}.build" ./cmd/agent ) || die "go build failed"

  install -m 0755 "/tmp/${BINARY_NAME}.build" "${PREFIX}/${BINARY_NAME}"
  rm -f "/tmp/${BINARY_NAME}.build"
  log "Installed binary: ${PREFIX}/${BINARY_NAME}"
}

# ---- 3. user, config, spool ----------------------------------------------
create_user_and_dirs() {
  if ! getent group "$SERVICE_USER" >/dev/null 2>&1; then
    log "Creating group '${SERVICE_USER}' ..."
    groupadd --system "$SERVICE_USER"
  fi
  if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
    log "Creating system user '${SERVICE_USER}' ..."
    useradd --system --no-create-home --gid "$SERVICE_USER" --shell /usr/sbin/nologin "$SERVICE_USER"
  else
    usermod -aG "$SERVICE_USER" "$SERVICE_USER" 2>/dev/null || true
  fi
  # The config dir must be traversable by the service group so the agent can
  # read its config file (owned root:storagebench). Group-owned by the service
  # user, mode 0750: root rwx, group r-x, others none.
  install -d -m 0750 -o root -g "$SERVICE_USER" "$CONFIG_DIR"
  install -d -m 0750 -o "$SERVICE_USER" -g "$SERVICE_USER" "$(dirname "$SPOOL_DIR")"
  install -d -m 0750 -o "$SERVICE_USER" -g "$SERVICE_USER" "$SPOOL_DIR"
}

write_config() {
  # Preserve an existing brokerSocket across re-runs so this installer never
  # silently disables fault injection. Precedence: explicit --broker-socket flag,
  # else the value already present in the config, else none.
  local broker="$BROKER_SOCKET"
  if [[ -z "$broker" && -f "$CONFIG_FILE" ]]; then
    broker="$(sed -n 's/.*"brokerSocket"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CONFIG_FILE" | head -n1)"
    [[ -n "$broker" ]] && log "Preserving existing brokerSocket: ${broker}"
  fi
  local broker_line=""
  if [[ -n "$broker" ]]; then
    broker_line="  \"brokerSocket\": \"${broker}\","$'\n'
  fi

  log "Writing config ${CONFIG_FILE} (token kept out of this file) ..."
  # Non-secret config on disk; the token is provided via the env file (0600).
  cat > "$CONFIG_FILE" <<JSON
{
${broker_line}  "controllerUrl": "${CONTROLLER_URL}",
  "agentId": "${AGENT_ID}",
  "insecureSkipVerify": ${INSECURE},
  "spoolDir": "${SPOOL_DIR}",
  "sampleIntervalMs": 1000,
  "reconnectMinMs": 1000,
  "reconnectMaxMs": 30000,
  "logLevel": "info"
}
JSON
  chmod 0640 "$CONFIG_FILE"
  chown root:"$SERVICE_USER" "$CONFIG_FILE"
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "import json,sys;json.load(open('${CONFIG_FILE}'))" \
      && log "Agent config is valid JSON." \
      || die "generated agent config is not valid JSON: ${CONFIG_FILE}"
  fi

  log "Writing secret env file ${ENV_FILE} (0600) ..."
  umask 077
  cat > "$ENV_FILE" <<ENV
# Secret for the storagebench agent. Readable only by root; loaded by systemd.
STORAGEBENCH_AGENT_TOKEN=${TOKEN}
ENV
  chmod 0600 "$ENV_FILE"
  chown root:root "$ENV_FILE"
}

# ---- 4. systemd service ---------------------------------------------------
write_service() {
  local unit="/etc/systemd/system/${SERVICE_NAME}.service"
  log "Installing systemd unit ${unit} ..."
  cat > "$unit" <<UNIT
[Unit]
Description=storagebench agent (benchmark node daemon)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
# Join the broker group so the agent can reach the privilege-broker socket
# (/run/storagebench/broker.sock, mode 0660 root:storagebench) when fault
# injection is enabled. Harmless if the broker is not installed.
SupplementaryGroups=storagebench
# Secret token comes from the env file; non-secret settings from the JSON config.
EnvironmentFile=${ENV_FILE}
ExecStart=${PREFIX}/${BINARY_NAME} --config ${CONFIG_FILE}
Restart=always
RestartSec=3
# Hardening: the agent needs to run fio against configured targets, so we keep
# filesystem access but drop unnecessary privileges.
NoNewPrivileges=true
ProtectSystem=full
ProtectControlGroups=true
ProtectKernelTunables=true
RestrictSUIDSGID=true
LockPersonality=true
# The spool dir must stay writable for crash-safe job/telemetry buffering.
ReadWritePaths=${SPOOL_DIR}

[Install]
WantedBy=multi-user.target
UNIT

  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
  systemctl restart "$SERVICE_NAME"
}

# ---- run ------------------------------------------------------------------
log "storagebench agent deploy"
log "  controller : ${CONTROLLER_URL}"
log "  agent-id   : ${AGENT_ID}"
log "  insecure   : ${INSECURE}"
log "  repo root  : ${REPO_ROOT}"

[[ "$INSTALL_DEPS" == "true" ]] && install_deps || warn "skipping apt dependency install (--no-deps)"
ensure_fio

install_fetched_or_built agent "${PREFIX}/${BINARY_NAME}"

create_user_and_dirs
write_config
write_service

log "Done. Service status:"
systemctl --no-pager --full status "$SERVICE_NAME" || true
cat <<TIP

Next steps:
  * Follow logs:    journalctl -u ${SERVICE_NAME} -f
  * Restart:        sudo systemctl restart ${SERVICE_NAME}
  * Edit config:    sudo nano ${CONFIG_FILE}   (then restart)
  * Rotate token:   sudo nano ${ENV_FILE}      (then restart)

The agent should now appear in the controller dashboard once connected.
TIP
