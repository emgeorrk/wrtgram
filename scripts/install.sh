#!/bin/sh
# wrtgram installer for OpenWrt.
#
#   sh -c "$(wget -qO- https://raw.githubusercontent.com/emgeorrk/wrtgram/main/scripts/install.sh)"
#
# Options:
#   --version vX.Y.Z   install a specific release (default: latest)
#   --url URL          download the release tarball from URL (or a local path) instead of GitHub
#   --no-wizard        do not run the interactive setup afterwards
#   --uninstall        remove wrtgram (keeps /etc/config/wrtgram)
#
# It downloads the prebuilt binary for this router's architecture, verifies
# its checksum, installs the procd service and runs `wrtgram setup`.
set -eu

REPO="emgeorrk/wrtgram"
VERSION=""
URL=""
WIZARD=1
TMP="/tmp/wrtgram-install"

while [ $# -gt 0 ]; do
	case "$1" in
		--version) VERSION="$2"; shift ;;
		--url) URL="$2"; shift ;;
		--no-wizard) WIZARD=0 ;;
		--uninstall) UNINSTALL=1 ;;
		-h|--help) sed -n '2,15p' "$0"; exit 0 ;;
		*) echo "unknown option: $1" >&2; exit 2 ;;
	esac
	shift
done

die() { echo "error: $*" >&2; exit 1; }
fetch() { # url-or-path dest
	case "$1" in
		/*) cp "$1" "$2" ;;
		*) if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$2" "$1"; else wget -qO "$2" "$1"; fi ;;
	esac
}

if [ "${UNINSTALL:-0}" = 1 ]; then
	/etc/init.d/wrtgram stop 2>/dev/null || true
	/etc/init.d/wrtgram disable 2>/dev/null || true
	rm -f /usr/bin/wrtgram /etc/init.d/wrtgram /etc/hotplug.d/dhcp/90-wrtgram /var/run/wrtgram.sock
	[ -L /usr/bin/tg-notify ] && rm -f /usr/bin/tg-notify
	echo "wrtgram removed. Config kept in /etc/config/wrtgram, state in /etc/wrtgram/."
	exit 0
fi

# --- architecture ----------------------------------------------------------
[ -r /etc/os-release ] || die "not an OpenWrt system (/etc/os-release missing)"
. /etc/os-release
ARCH="${OPENWRT_ARCH:-}"
[ -n "$ARCH" ] || die "OPENWRT_ARCH not set in /etc/os-release"

case "$ARCH" in
	aarch64_*) ARTIFACT=linux_arm64 ;;
	arm_cortex-a7*|arm_cortex-a8*|arm_cortex-a9*|arm_cortex-a15*|arm_cortex-a5*) ARTIFACT=linux_armv7 ;;
	arm_arm926ej-s|arm_arm1176jzf-s_vfp|arm_xscale|arm_fa526|arm_mpcore*) ARTIFACT=linux_armv5 ;;
	mipsel_*) ARTIFACT=linux_mipsle ;;
	mips_*) ARTIFACT=linux_mips ;;
	x86_64) ARTIFACT=linux_amd64 ;;
	i386_*) ARTIFACT=linux_386 ;;
	riscv64*) ARTIFACT=linux_riscv64 ;;
	*) die "no prebuilt binary for architecture $ARCH (build from source with GOARCH set)" ;;
esac

# --- release ---------------------------------------------------------------
if [ -z "$URL" ]; then
	if [ -z "$VERSION" ]; then
		fetch "https://api.github.com/repos/$REPO/releases/latest" "$TMP.json" 2>/dev/null \
			|| die "cannot reach api.github.com (use --version or --url)"
		VERSION=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$TMP.json" | head -1)
		rm -f "$TMP.json"
		[ -n "$VERSION" ] || die "cannot determine the latest release"
	fi
	VER="${VERSION#v}"
	BASE="https://github.com/$REPO/releases/download/$VERSION"
	URL="$BASE/wrtgram-$VER-$ARTIFACT.tar.gz"
	SUMS="$BASE/checksums.txt"
else
	SUMS=""
fi

echo "wrtgram: arch $ARCH -> $ARTIFACT, ${VERSION:-custom build}"

# --- download and verify ---------------------------------------------------
rm -rf "$TMP"; mkdir -p "$TMP"
TARBALL="$TMP/wrtgram.tar.gz"
fetch "$URL" "$TARBALL" || die "download failed: $URL"

if [ -n "$SUMS" ]; then
	fetch "$SUMS" "$TMP/checksums.txt" || die "cannot download checksums.txt"
	WANT=$(grep " wrtgram-$VER-$ARTIFACT.tar.gz\$" "$TMP/checksums.txt" | cut -d' ' -f1)
	GOT=$(sha256sum "$TARBALL" | cut -d' ' -f1)
	[ -n "$WANT" ] && [ "$WANT" = "$GOT" ] || die "checksum mismatch for $URL"
	echo "wrtgram: checksum ok"
fi

SIZE=$(wc -c < "$TARBALL")
AVAIL=$(df -k /overlay 2>/dev/null | awk 'NR==2 {print $4 * 1024}')
[ -z "$AVAIL" ] && AVAIL=$(df -k / | awk 'NR==2 {print $4 * 1024}')
[ "$AVAIL" -gt $((SIZE * 3)) ] || die "not enough free space: $((AVAIL / 1024)) KiB available, need about $((SIZE * 3 / 1024)) KiB"

tar -xzf "$TARBALL" -C "$TMP" || die "cannot extract $TARBALL"
[ -x "$TMP/wrtgram" ] || die "tarball has no wrtgram binary"

# --- install ---------------------------------------------------------------
if [ -x /etc/init.d/wrtgram ]; then
	/etc/init.d/wrtgram stop 2>/dev/null || true
fi

cp "$TMP/wrtgram" /usr/bin/wrtgram && chmod 755 /usr/bin/wrtgram
cp "$TMP/files/wrtgram.init" /etc/init.d/wrtgram && chmod 755 /etc/init.d/wrtgram
mkdir -p /etc/hotplug.d/dhcp
cp "$TMP/files/wrtgram.hotplug" /etc/hotplug.d/dhcp/90-wrtgram && chmod 755 /etc/hotplug.d/dhcp/90-wrtgram
if [ ! -s /etc/config/wrtgram ]; then
	cp "$TMP/files/wrtgram.config" /etc/config/wrtgram
fi
sh "$TMP/files/wrtgram.defaults"
grep -qx '/etc/wrtgram/' /etc/sysupgrade.conf 2>/dev/null || echo '/etc/wrtgram/' >> /etc/sysupgrade.conf
/etc/init.d/wrtgram enable
rm -rf "$TMP"

echo "wrtgram: installed $(/usr/bin/wrtgram version)"
/usr/bin/wrtgram probe 2>/dev/null | sed -n '/^Modules/,/^$/p' || true

# --- setup -----------------------------------------------------------------
TOKEN_SET=$(uci -q get wrtgram.main.token || true)
if [ "$WIZARD" = 1 ] && [ -z "$TOKEN_SET" ] && [ -t 0 ]; then
	/usr/bin/wrtgram setup
else
	/etc/init.d/wrtgram start
	if [ -z "$TOKEN_SET" ]; then
		echo
		echo "Next: run 'wrtgram setup' to enter the bot token, or edit /etc/config/wrtgram"
		echo "and run '/etc/init.d/wrtgram restart'."
	fi
fi
