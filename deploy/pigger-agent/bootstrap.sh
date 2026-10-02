#!/bin/sh
set -eu

version='@@VERSION@@'
if [ "$#" -ne 2 ]; then
  echo 'usage: install <panel URL> <host secret>' >&2
  exit 2
fi
master=${1%/}/
secret=$2
case "$master" in http://*|https://*) ;; *) echo 'Invalid panel URL' >&2; exit 2 ;; esac
case "$secret" in ''|*[!a-zA-Z0-9_-]*) echo 'Invalid host secret' >&2; exit 2 ;; esac
if [ "$(id -u)" != 0 ]; then echo 'Run the install command as root' >&2; exit 1; fi
case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) arch=amd64 ;;
  Linux/aarch64|Linux/arm64) arch=arm64 ;;
  *) echo 'Supported hosts: Linux amd64 and arm64' >&2; exit 1 ;;
esac
for cmd in curl tar sha256sum install; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Install $cmd first" >&2; exit 1; }
done
if ! { command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; } && ! command -v rc-service >/dev/null 2>&1; then
  echo 'This installer requires systemd or OpenRC' >&2
  exit 1
fi
umask 077
work=$(mktemp -d)
cleanup() {
  rm -f "$work/archive.tar.gz" "$work/archive.sha256" "$work/pigger-agent" "$work/install.sh" "$work/pigger-agent.service" "$work/pigger-agent.openrc" "$work/geoip.dat" "$work/geosite.dat"
  rmdir "$work"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
download() {
  curl --proto '=http,https' --connect-timeout 15 --retry 2 -fsS \
    -H "Authorization: Bearer $secret" "${master}agent/download/$version/$1" -o "$work/$2"
}
download "linux-$arch.tar.gz" archive.tar.gz
download "linux-$arch.sha256" archive.sha256
(cd "$work" && sha256sum -c archive.sha256)
tar --no-same-owner -xzf "$work/archive.tar.gz" -C "$work"
if [ "$("$work/pigger-agent" -version)" != "$version" ]; then
  echo 'The downloaded agent does not match the panel version' >&2
  exit 1
fi
download geoip.dat geoip.dat
download geosite.dat geosite.dat
sh "$work/install.sh" "$master" "$secret"
