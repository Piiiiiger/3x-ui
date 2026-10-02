#!/bin/sh
# Installs pigger-agent from this directory. Run it as root next to the
# pigger-agent binary and the geoip.dat/geosite.dat its Xray should use.
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <panel URL with its base path> <agent secret>" >&2
	exit 2
fi
master=$1
secret=$2
case "$master$secret" in
*\"* | *\\* | *[[:space:]]*)
	echo "the panel URL and the secret must not contain quotes, backslashes or spaces" >&2
	exit 2
	;;
esac
here=$(cd "$(dirname "$0")" && pwd)

install -d -m 755 /usr/local/pigger-agent
install -m 755 "$here/pigger-agent" /usr/local/pigger-agent/pigger-agent
for f in geoip.dat geosite.dat; do
	if [ -f "$here/$f" ]; then
		install -m 644 "$here/$f" "/usr/local/pigger-agent/$f"
	fi
done
install -d -m 700 /etc/pigger-agent /var/lib/pigger-agent
install -d -m 750 /var/log/pigger-agent
(
	umask 077
	printf '{"master": "%s", "secret": "%s"}\n' "$master" "$secret" >/etc/pigger-agent/config.json
)

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
	install -m 644 "$here/pigger-agent.service" /etc/systemd/system/pigger-agent.service
	systemctl daemon-reload
	systemctl enable pigger-agent
	systemctl restart pigger-agent
elif command -v rc-service >/dev/null 2>&1; then
	install -m 755 "$here/pigger-agent.openrc" /etc/init.d/pigger-agent
	rc-update add pigger-agent default
	rc-service pigger-agent restart
else
	echo "neither systemd nor OpenRC found; run /usr/local/pigger-agent/pigger-agent yourself" >&2
	exit 1
fi
echo "pigger-agent installed and started; it dials $master"
