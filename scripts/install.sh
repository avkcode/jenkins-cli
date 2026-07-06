#!/usr/bin/env bash
set -euo pipefail

REPO="${AERO_CI_REPO:-avkcode/jenkins-cli}"
VERSION="${AERO_CI_VERSION:-latest}"
INSTALL_DIR="${AERO_CI_INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="${AERO_CI_BINARY_NAME:-jc}"

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

base_url="https://github.com/${REPO}/releases"
if [ "$VERSION" = "latest" ]; then
  download_url="${base_url}/latest/download"
else
  download_url="${base_url}/download/${VERSION}"
fi

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT

asset="jc-${os}-${arch}"
echo "Downloading ${asset} from ${download_url}"
curl -fsSL "${download_url}/${asset}" -o "${tmp}/jc"
chmod 0755 "${tmp}/jc"

if curl -fsSL "${download_url}/checksums.txt" -o "${tmp}/checksums.txt"; then
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmp" && grep " ${asset}$" checksums.txt | sed "s/ ${asset}$/ jc/" | sha256sum -c -)
  elif command -v shasum >/dev/null 2>&1; then
    expected="$(grep " ${asset}$" "${tmp}/checksums.txt" | awk '{print $1}')"
    actual="$(shasum -a 256 "${tmp}/jc" | awk '{print $1}')"
    if [ "$expected" != "$actual" ]; then
      echo "Checksum mismatch for ${asset}" >&2
      exit 1
    fi
  fi
fi

if [ -w "$INSTALL_DIR" ]; then
  install -m 0755 "${tmp}/jc" "${INSTALL_DIR}/${BINARY_NAME}"
else
  sudo install -m 0755 "${tmp}/jc" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
echo "Run: ${BINARY_NAME} --version"
