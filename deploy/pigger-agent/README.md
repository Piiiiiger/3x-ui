# pigger-agent

A Pigger panel can run a server as an **agent node** instead of installing a full
panel there. The agent is one small program with Xray built in: it dials the
panel over a WebSocket, runs the Xray config the panel sends, and reports
traffic, online clients and the server's load back. The panel keeps every
user, quota and setting; only the configured Xray and optional Snell listeners are exposed on the server.

## Install on a server

1. In the panel, add a node with **Kind: Agent** and the server's public
   address, then copy the secret the panel shows (it is shown once).
2. Build the agent (`make build-agent`) and copy `pigger-agent`, `install.sh`,
   `pigger-agent.service`, `pigger-agent.openrc` and Xray's `geoip.dat` and
   `geosite.dat` into one directory on the server.
3. As root, in that directory:

       sh install.sh https://panel.example.com/<base path>/ <secret>

The script installs to `/usr/local/pigger-agent`, writes
`/etc/pigger-agent/config.json`, and starts the service under systemd or
OpenRC (Alpine). The agent keeps its state in `/var/lib/pigger-agent` and logs
to `/var/log/pigger-agent`.

## Notes

- The agent starts Xray from the last config it received, so users stay up
  while the panel is unreachable; traffic counted meanwhile is reported when
  it reconnects.
- Minting a new secret in the panel disconnects the agent that used the old
  one; update `/etc/pigger-agent/config.json` and restart the service.
- MTProto, TUIC and AmneziaWG inbounds need a full panel node.

## Managed Snell v5 (systemd)

Install an official Snell v5 binary, verified for the server architecture, at
/usr/local/libexec/pigger-agent/snell-server with root ownership and mode 0755,
then restart the updated agent. It advertises snell-v5-systemd only when the
binary reports version 5 and systemd is present. Inbounds cannot supply executable paths.

Select Snell when adding an inbound on an Agent host. The panel manages its
listen address, TCP/UDP port, shared PSK, enable state and absolute expiry.
Assign through a plan and use a Clash/Mihomo subscription supporting Snell.
Adding an inbound never creates personal customizations or assigns subscribers.
Xray JSON and ordinary share-link exports do not support this protocol.

Each inbound gets a pigger-snell-<inbound id>.service systemd unit. Credentials
and state live under <agent stateDir>/snell/ with root-only permissions;
systemd supplies a private credential to a DynamicUser process. The panel
shows service state reported by the agent. Failed updates restore the previous
managed configuration. Disconnecting the agent does not stop listeners;
absolute expiry is enforced while the agent runs. Standalone services are never
adopted automatically: stop the specific old listener before enabling its
replacement, and keep the original configuration for rollback.

Snell uses a shared PSK, not individual panel user credentials, so it cannot
tell users apart. The agent counts each Snell port's traffic with nftables
counters in a table of its own (`inet pigger_snell`, counters only, accepts
everything) and reports it as the inbound's traffic; without `nft`, Snell runs
uncounted. When exactly one user has access to a Snell inbound, its traffic
counts toward that user's usage and quota, and the panel stops the listener
while that user is disabled, expired or out of traffic. Shared by several users,
its traffic counts for the host only, and IP limits and per-user revocation
cannot be enforced. Rotate the PSK to revoke previously issued access; existing
clients must refresh their configurations.
