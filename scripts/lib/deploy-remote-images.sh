#!/usr/bin/env bash
# shellcheck shell=bash
# Remote image build + k3s import helpers (sourced by deploy-remote.sh on the build host).

set -euo pipefail
export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${PATH:-}"

AXIOM_IMAGE_TAG="${AXIOM_IMAGE_TAG:-dev}"

axiom_k3s_ctr() {
  if [ -n "${AXIOM_K3S_CTR_CMD:-}" ]; then
    "${AXIOM_K3S_CTR_CMD[@]}" "$@"
    return
  fi
  if [ -x /usr/local/bin/k3s ]; then
    sudo /usr/local/bin/k3s ctr "$@"
  else
    sudo k3s ctr "$@"
  fi
}

axiom_detect_k3s_ctr() {
  if [ -x /usr/local/bin/k3s ] && sudo /usr/local/bin/k3s ctr images ls &>/dev/null; then
    AXIOM_K3S_CTR_CMD=(sudo /usr/local/bin/k3s ctr)
  elif command -v k3s &>/dev/null && sudo k3s ctr images ls &>/dev/null; then
    AXIOM_K3S_CTR_CMD=(sudo k3s ctr)
  elif command -v k3s &>/dev/null && k3s ctr images ls &>/dev/null; then
    AXIOM_K3S_CTR_CMD=(k3s ctr)
  else
    return 1
  fi
}

axiom_container_cmd() {
  if command -v podman &>/dev/null; then
    echo podman
  elif command -v docker &>/dev/null; then
    echo docker
  else
    return 1
  fi
}

axiom_import_image() {
  local tag="$1"
  local ctr="$2"
  local tmp
  echo "  Importing ${tag} into k3s..."
  for ref in "docker.io/library/${tag}" "docker.io/${tag}" "${tag}" "localhost/${tag}"; do
    axiom_k3s_ctr images rm "${ref}" 2>/dev/null || true
  done
  # File-based import avoids sudo consuming stdin from a pipe (hangs on many hosts).
  tmp=$(mktemp /tmp/axiom-img-XXXXXX.tar)
  "$ctr" save -o "$tmp" "$tag"
  axiom_k3s_ctr images import "$tmp"
  rm -f "$tmp"
  axiom_k3s_ctr images tag "localhost/${tag}" "docker.io/library/${tag}" 2>/dev/null || true
  axiom_k3s_ctr images tag "localhost/${tag}" "docker.io/${tag}" 2>/dev/null || true
  axiom_k3s_ctr images tag "localhost/${tag}" "${tag}" 2>/dev/null || true
}

axiom_build_image() {
  local ctr="$1"
  local tag="zyvor-axiom:${AXIOM_IMAGE_TAG}"
  echo "  Building ${tag}..."
  "$ctr" build -t "$tag" -f Dockerfile .
  axiom_import_image "$tag" "$ctr"
}

axiom_build_all_images() {
  local ctr
  ctr="$(axiom_container_cmd)" || {
    echo "MISSING: podman or docker required to build images"
    return 1
  }
  axiom_detect_k3s_ctr || {
    echo "MISSING: k3s ctr required to import images (tried sudo /usr/local/bin/k3s ctr)"
    return 1
  }
  echo "  Container runtime: ${ctr}"
  axiom_build_image "$ctr"
  echo "  Image ready: zyvor-axiom:${AXIOM_IMAGE_TAG}"
}
