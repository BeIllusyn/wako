#!/bin/sh
# Install wako from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/liuenzuo666/wako/master/install.sh | sh
#
# Environment variables:
#   WAKO_VERSION  version to install, for example v1.0.0 (default: latest release)
#   WAKO_INSTALL  directory to install into (default: ~/.local/bin)

set -eu

repo="liuenzuo666/wako"
bin="wako"
version="${WAKO_VERSION:-}"
install_dir="${WAKO_INSTALL:-$HOME/.local/bin}"

err() {
	printf 'wako-install: %s\n' "$*" >&2
	exit 1
}

for tool in curl tar uname; do
	command -v "$tool" >/dev/null 2>&1 || err "required command not found: $tool"
done

os="$(uname -s)"
case "$os" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) err "unsupported operating system: $os (download a release from https://github.com/$repo/releases)" ;;
esac

arch="$(uname -m)"
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture: $arch" ;;
esac

if [ -z "$version" ]; then
	latest="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")"
	case "$latest" in
	*/releases/tag/*) version="${latest##*/tag/}" ;;
	*) err "no releases found; see https://github.com/$repo/releases" ;;
	esac
fi
case "$version" in
v*) ;;
*) version="v$version" ;;
esac

name="${bin}_${version#v}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$version"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Downloading %s...\n' "$base/$name.tar.gz"
curl -fsSL --retry 3 -o "$tmp/$name.tar.gz" "$base/$name.tar.gz" ||
	err "download failed (does $version have a $os/$arch build?)"

# Verify the archive against the release checksums when they are available.
if curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/checksums.txt" 2>/dev/null; then
	if command -v sha256sum >/dev/null 2>&1; then
		(cd "$tmp" && sha256sum -c --ignore-missing checksums.txt >/dev/null) ||
			err "checksum verification failed for $name.tar.gz"
	elif command -v shasum >/dev/null 2>&1; then
		(cd "$tmp" && shasum -a 256 -c --ignore-missing checksums.txt >/dev/null) ||
			err "checksum verification failed for $name.tar.gz"
	fi
fi

tar -xzf "$tmp/$name.tar.gz" -C "$tmp" "$bin" || err "could not extract $name.tar.gz"

mkdir -p "$install_dir"
cp "$tmp/$bin" "$install_dir/$bin"
chmod 755 "$install_dir/$bin"

printf 'Installed %s %s to %s\n' "$bin" "$version" "$install_dir/$bin"
case ":$PATH:" in
*":$install_dir:"*) ;;
*) printf '\n%s is not in your PATH; add it with:\n  export PATH="%s:$PATH"\n' "$install_dir" "$install_dir" ;;
esac
