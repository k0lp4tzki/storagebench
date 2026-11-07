# fetch-prebuilt.sh — download a CI-built agent/broker binary so hosts never
# have to compile. Sourced by the install-*.sh scripts (expects log/warn/die).
#
# Sources, in the order the caller chooses via FETCH_SOURCE:
#   release  GitHub Release "nightly" (no Docker required)
#   image    extract the binary from the GHCR image (requires docker)
#
# Override defaults with env:
#   STORAGEBENCH_GITHUB_REPO   owner/name (default: k0lp4tzki/storagebench)
#   STORAGEBENCH_RELEASE       release tag (default: nightly)
#   STORAGEBENCH_GHCR          registry prefix (default: ghcr.io/k0lp4tzki)
#   GITHUB_TOKEN / STORAGEBENCH_GITHUB_TOKEN   for private repos
#
# shellcheck disable=SC2034

STORAGEBENCH_GITHUB_REPO="${STORAGEBENCH_GITHUB_REPO:-k0lp4tzki/storagebench}"
STORAGEBENCH_RELEASE="${STORAGEBENCH_RELEASE:-nightly}"
STORAGEBENCH_GHCR="${STORAGEBENCH_GHCR:-ghcr.io/k0lp4tzki}"

# fetch_release_binary KIND DEST
# KIND is agent|broker. Downloads storagebench-KIND-linux-amd64 from the nightly
# GitHub Release.
fetch_release_binary() {
  local kind="$1" dest="$2"
  local asset="storagebench-${kind}-linux-amd64"
  local url="https://github.com/${STORAGEBENCH_GITHUB_REPO}/releases/download/${STORAGEBENCH_RELEASE}/${asset}"
  local token="${STORAGEBENCH_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"
  local tmp
  tmp="$(mktemp)"
  log "Downloading prebuilt ${kind} from GitHub Release ${STORAGEBENCH_RELEASE} ..."
  log "  ${url}"
  local -a curl_auth=()
  if [[ -n "$token" ]]; then
    curl_auth=(-H "Authorization: Bearer ${token}")
  fi
  if ! curl -fL --retry 3 --retry-delay 2 "${curl_auth[@]}" -o "$tmp" "$url"; then
    rm -f "$tmp"
    die "download failed. Wait for CI to publish the '${STORAGEBENCH_RELEASE}' release after a push to main, or pass --build to compile locally. For a private repo set GITHUB_TOKEN."
  fi
  install -m 0755 "$tmp" "$dest"
  rm -f "$tmp"
  log "Installed binary: ${dest}"
}

# fetch_image_binary KIND DEST
# Pulls ghcr.io/.../storagebench-KIND:latest and copies the binary out. The host
# keeps running systemd (not the container); Docker is only used as a fetch.
fetch_image_binary() {
  local kind="$1" dest="$2"
  local image="${STORAGEBENCH_GHCR}/storagebench-${kind}:latest"
  command -v docker >/dev/null 2>&1 || die "docker is required for --from-image (or use the default --from-release)."
  log "Pulling ${image} ..."
  if ! docker pull "$image"; then
    die "docker pull failed. For a private GHCR package: echo \$GITHUB_TOKEN | docker login ghcr.io -u USER --password-stdin"
  fi
  local cid
  cid="$(docker create "$image")"
  docker cp "${cid}:/usr/local/bin/storagebench-${kind}" "$dest"
  docker rm "$cid" >/dev/null
  chmod 0755 "$dest"
  log "Extracted binary from image: ${dest}"
}

# install_fetched_or_built KIND DEST
# KIND=agent|broker. Honours BINARY_SRC, FETCH_SOURCE, DO_BUILD. The caller
# must define build_agent or build_broker.
install_fetched_or_built() {
  local kind="$1" dest="$2"
  if [[ -n "${BINARY_SRC:-}" ]]; then
    [[ -f "$BINARY_SRC" ]] || die "--binary set but file not found: ${BINARY_SRC}"
    log "Installing prebuilt binary from ${BINARY_SRC} (no build) ..."
    install -m 0755 "$BINARY_SRC" "$dest"
    log "Installed binary: ${dest}"
  elif [[ "${FETCH_SOURCE:-}" == "release" ]]; then
    fetch_release_binary "$kind" "$dest"
  elif [[ "${FETCH_SOURCE:-}" == "image" ]]; then
    fetch_image_binary "$kind" "$dest"
  elif [[ "${DO_BUILD:-}" == "true" ]]; then
    if [[ "$kind" == "agent" ]]; then
      build_agent
    else
      build_broker
    fi
  else
    [[ -x "$dest" ]] || die "--no-build set but ${dest} is not present/executable."
    warn "skipping build (--no-build); using existing ${dest}"
  fi
}
