**English** | [简体中文](README.zh_CN.md)

# ProxyPigger

ProxyPigger (Pigger for short) is a self-hosted panel for running a small proxy
service: a handful of servers and the people who use them. One panel holds every
user, plan, quota and rule; each server runs a small agent with Xray built in.

It started as a fork of [3X-UI](https://github.com/MHSanaei/3x-ui) and has since
been rebuilt around hosts, users, plans and rule templates, with its own agent,
user portal and look.

## Features

- **Hosts and nodes.** Each server is a *host* with the *nodes* (inbounds) it
  runs. Generate the next node on a host in one step, with fresh REALITY keys
  and a free port, and set a separate public port where a relay or NAT sits in
  front.
- **pigger-agent.** A single static binary with Xray embedded instead of a full
  panel on every server. It dials the panel over a WebSocket, runs the config
  the panel sends, and reports traffic, online users and load. If the panel is
  unreachable it keeps serving from the last config. Runs under systemd or
  OpenRC (Alpine), and can manage Snell v5 listeners.
- **Users.** Quota, expiry and reset day belong to each user. Activation codes,
  Telegram binding for usage reports and expiry reminders, and a list of users
  who have run out, split by cause.
- **Plans.** A plan is a set of servers, a rule set and an IP limit. A node can
  belong to several plans, and plan changes reach every member.
- **Rules.** Clash / Mihomo rule templates: one base rule set, with variants
  that hold only their differences. Preview the result and see who uses each.
- **Subscriptions and portal.** Clash YAML rendered from the templates, plus
  share links. Users log in to a portal to see their usage, copy their
  subscription, upload custom rules and check the status of their own servers.
- **Traffic overview.** Totals and daily history per user and per host, and the
  users who need attention.
- **Probe.** Server cards (CPU, memory, disk, ping per carrier) read from a
  [Lite](https://github.com/nuomiiiii/Lite) monitor on the same machine. In the
  portal each user sees only the servers in their subscription. Agents can be
  installed straight onto servers the monitor already knows.
- **Proxy chains.** Managed relay → landing chains.
- **Interface.** Flat coral theme in light and dark, with a top navigation bar.
  Simplified Chinese and English.

Removed from 3X-UI: client groups, the outbounds and routing pages, and the API
docs and sponsors pages.

## Install

There are no ProxyPigger release packages yet. The panel binary is a drop-in
replacement for 3X-UI's `x-ui`, so the tested route is:

1. Install 3X-UI v3.8.5 on the server with its own installer.
2. Build ProxyPigger (below) and replace `/usr/local/x-ui/x-ui` with the new
   binary.
3. Restart the service: `systemctl restart x-ui`.

> [!WARNING]
> Never use 3X-UI's **update** button, `x-ui update` or 3X-UI's `install.sh`
> on a ProxyPigger server: they install upstream 3X-UI over it.
>
> Back up the database before the first start and before every upgrade.
> ProxyPigger's migrations drop tables for removed features, so going back
> needs the backup. The SQLite database runs in WAL mode; copy it with
> `sqlite3 /etc/x-ui/x-ui.db ".backup /root/x-ui-backup.db"`, not `cp`.

To add servers as agent nodes, see [deploy/pigger-agent](deploy/pigger-agent/README.md).

## Build

Requirements: Go 1.27.1 or newer, Node.js 26 (`.nvmrc`), and a C compiler (cgo,
for SQLite).

```bash
git clone https://github.com/Piiiiiger/Proxypigger.git
cd Proxypigger
make build-fe build-agent-package   # frontend bundle and the agent packages the panel serves
go build -o x-ui .                  # the panel
make build-agent                    # pigger-agent on its own
```

## Development

- `make help` lists the tasks. `make verify` runs the full gate: code generation
  check, linters, formatter, type check, Go and frontend tests, build and
  Storybook.
- [docs/architecture.md](docs/architecture.md) maps the code.
- [CLAUDE.md](CLAUDE.md) and [CONTRIBUTING.md](CONTRIBUTING.md) hold the
  conventions.

Some upstream names remain on purpose so existing installs keep working: the
binary and service are `x-ui`, data lives in `/etc/x-ui`, environment variables
start with `XUI_`, and the Go module path is still `github.com/mhsanaei/3x-ui/v3`.

## Credits

ProxyPigger is built on [3X-UI](https://github.com/MHSanaei/3x-ui) by MHSanaei
and its contributors, which grew out of [X-UI](https://github.com/vaxilu/x-ui),
and on [Xray-core](https://github.com/XTLS/Xray-core). Server status comes from
[Lite](https://github.com/nuomiiiii/Lite).

## License

[GPL-3.0](LICENSE), the same as 3X-UI.
