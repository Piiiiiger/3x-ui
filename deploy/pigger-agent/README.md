# pigger-agent

A Pigger panel can run a server as an **agent node** instead of installing a full
panel there. The agent is one small program with Xray built in: it dials the
panel over a WebSocket, runs the Xray config the panel sends, and reports
traffic, online clients and the server's load back. The panel keeps every
user, quota and setting; nothing listens on the server except Xray itself.

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
