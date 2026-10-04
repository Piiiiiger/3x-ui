[English](README.md) | **简体中文**

# ProxyPigger

ProxyPigger（简称 Pigger）是一个自托管的面板，用来运营一个小型代理服务：几台服务器，加上使用它们的人。
所有用户、套餐、流量额度和规则都在一个面板里管理；每台服务器只运行一个内置 Xray 的小型 agent。

它最初是 [3X-UI](https://github.com/MHSanaei/3x-ui) 的分支，之后围绕主机、用户、套餐和规则模板重新构建，
有了自己的 agent、用户门户和界面风格。

## 功能

- **主机与节点。** 每台服务器是一个*主机*，上面运行若干*节点*（入站）。一键在主机上生成下一个节点，
  自动生成新的 REALITY 密钥并挑选空闲端口；前面有中转或 NAT 时，可以单独设置对外端口。
- **pigger-agent。** 不必在每台服务器上装完整面板，只需一个内置 Xray 的静态二进制。
  它通过 WebSocket 连接面板，运行面板下发的配置，并回报流量、在线用户和负载。
  面板暂时连不上时，它会继续用最后一份配置提供服务。支持 systemd 和 OpenRC（Alpine），也可以托管 Snell v5。
- **用户。** 流量额度、到期时间和重置日属于每个用户。支持激活码、绑定 Telegram 接收用量报告和到期提醒，
  并按原因列出已用尽的用户。
- **套餐。** 一个套餐由一组服务器、一套规则和一个 IP 限制组成。一个节点可以属于多个套餐，
  套餐的改动会同步到所有成员。
- **规则。** Clash / Mihomo 规则模板：一套基础规则，变体只保存与基础规则的差异。可以预览结果，并查看谁在使用。
- **订阅与门户。** 由模板生成 Clash YAML，也提供分享链接。用户登录门户即可查看用量、复制订阅、
  上传自定义规则，并查看自己服务器的状态。
- **流量信息。** 按用户和按主机统计的总量与每日历史，以及需要关注的用户。
- **探针。** 服务器卡片（CPU、内存、磁盘、三网延迟）读取自同一台机器上的
  [Lite](https://github.com/nuomiiiii/Lite) 监控。在门户中，每个用户只能看到自己订阅里的服务器。
  也可以直接在监控已知的服务器上安装 agent。
- **代理链。** 管理"中转 → 落地"链路。
- **界面。** 珊瑚色扁平主题，支持浅色和深色，顶部导航栏。支持简体中文和英文。

相比 3X-UI 移除了：客户端分组、出站和路由页面，以及 API 文档和赞助页面。

## 安装

ProxyPigger 目前还没有发布安装包。面板二进制可以直接替换 3X-UI 的 `x-ui`，经过验证的方式是：

1. 用 3X-UI 自己的安装脚本在服务器上安装 3X-UI v3.8.5。
2. 构建 ProxyPigger（见下文），用新的二进制替换 `/usr/local/x-ui/x-ui`。
3. 重启服务：`systemctl restart x-ui`。

> [!WARNING]
> 不要在 ProxyPigger 服务器上使用 3X-UI 的**更新**按钮、`x-ui update` 或 3X-UI 的 `install.sh`：
> 它们会用上游 3X-UI 覆盖掉 ProxyPigger。
>
> 首次启动前和每次升级前都要备份数据库。ProxyPigger 的迁移会删除已移除功能对应的数据表，
> 回退时需要用到备份。SQLite 数据库运行在 WAL 模式下，请用
> `sqlite3 /etc/x-ui/x-ui.db ".backup /root/x-ui-backup.db"` 备份，不要用 `cp`。

把服务器作为 agent 节点加入面板，请看 [deploy/pigger-agent](deploy/pigger-agent/README.md)。

## 构建

需要 Go 1.27.1 或更新版本、Node.js 26（见 `.nvmrc`），以及 C 编译器（SQLite 需要 cgo）。

```bash
git clone https://github.com/Piiiiiger/Proxypigger.git
cd Proxypigger
make build-fe build-agent-package   # 前端资源，以及面板提供下载的 agent 安装包
go build -o x-ui .                  # 面板
make build-agent                    # 单独构建 pigger-agent
```

## 开发

- `make help` 列出所有任务。`make verify` 运行完整检查：代码生成检查、lint、格式化、类型检查、
  Go 和前端测试、构建以及 Storybook。
- [docs/architecture.md](docs/architecture.md) 是代码地图。
- [CLAUDE.md](CLAUDE.md) 和 [CONTRIBUTING.md](CONTRIBUTING.md) 记录了开发约定。

为了让现有安装继续工作，一些上游名称被有意保留：二进制和服务名是 `x-ui`，数据在 `/etc/x-ui`，
环境变量以 `XUI_` 开头，Go 模块路径仍是 `github.com/mhsanaei/3x-ui/v3`。

## 致谢

ProxyPigger 基于 MHSanaei 及其贡献者的 [3X-UI](https://github.com/MHSanaei/3x-ui)
（它源自 [X-UI](https://github.com/vaxilu/x-ui)），以及 [Xray-core](https://github.com/XTLS/Xray-core)。
服务器状态来自 [Lite](https://github.com/nuomiiiii/Lite)。

## 许可证

[GPL-3.0](LICENSE)，与 3X-UI 相同。
