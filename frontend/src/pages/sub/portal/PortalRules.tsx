import { useState } from 'react';
import { Alert, Button, Modal, Typography } from 'antd';
import { EnvironmentOutlined, ReadOutlined } from '@ant-design/icons';

// The snooze is the reader's own convenience, so it lives in their browser;
// without storage the rules simply show on every visit.
const SNOOZE_KEY = 'pigger.portal.rules.snoozedUntil';
const SNOOZE_MS = 7 * 24 * 60 * 60 * 1000;

function snoozedUntil(): number {
  try {
    return Number(window.localStorage.getItem(SNOOZE_KEY)) || 0;
  } catch {
    return 0;
  }
}

export function snoozePortalRules() {
  try {
    window.localStorage.setItem(SNOOZE_KEY, String(Date.now() + SNOOZE_MS));
  } catch {
    // Nothing to remember it in: the rules open again next visit.
  }
}

export default function PortalRules({ limitIp }: { limitIp?: number }) {
  const [open, setOpen] = useState(() => Date.now() >= snoozedUntil());
  const sharing =
    limitIp && limitIp > 0
      ? `分享账号：同一时间最多 ${limitIp} 个 IP 在线`
      : '分享账号：同一时间在线的 IP 数有上限';
  return (
    <>
      <Button size="large" icon={<ReadOutlined />} onClick={() => setOpen(true)}>
        使用须知
      </Button>
      {open && (
        <Modal
          title="使用须知"
          open
          onCancel={() => setOpen(false)}
          footer={[
            <Button
              key="snooze"
              onClick={() => {
                snoozePortalRules();
                setOpen(false);
              }}
            >
              7 天内不再提示
            </Button>,
            <Button key="ok" type="primary" onClick={() => setOpen(false)}>
              我知道了
            </Button>,
          ]}
        >
          <Alert
            type="error"
            showIcon
            icon={<EnvironmentOutlined />}
            style={{ marginBottom: 16 }}
            title={
              <span style={{ fontSize: 18, fontWeight: 700 }}>
                手机请务必关闭定位服务，防止节点「送中」
              </span>
            }
            description={
              <>
                <div>
                  开着定位时，Google 会读到手机的真实位置，把你正在用的节点 IP
                  判定为中国大陆，也就是「送中」；代理改变不了定位。节点被送中后，所有人用它访问
                  Google、YouTube、Gemini 等都会受影响。
                </div>
                <div>iPhone：设置 → 隐私与安全性 → 定位服务，关闭。</div>
                <div>安卓：下拉快捷开关，关闭「位置信息」。</div>
              </>
            }
          />
          <Typography.Title level={5}>禁止以下行为（系统自动检测）</Typography.Title>
          <ul>
            <li>批量注册账号</li>
            <li>爬虫、大规模抓取网页</li>
            <li>长时间持续满速占用带宽</li>
            <li>无故反复测速</li>
            <li>端口扫描、网络攻击、发送垃圾邮件、BT 下载</li>
            <li>{sharing}</li>
          </ul>
          <Typography.Paragraph type="secondary">
            正常下载软件、系统更新不受影响。连续满速 2 小时会收到提醒，持续 4 小时才会封禁。
          </Typography.Paragraph>
          <Typography.Title level={5}>处罚</Typography.Title>
          <ul>
            <li>每次违规封禁 30 分钟，到时自动恢复。</li>
            <li>30 天内违规超过 3 次，账号停用，需联系管理员说明情况。</li>
            <li>同时在线 IP 超出上限只会暂停超出的 IP 30 分钟，不计入违规次数。</li>
            <li>封禁和提醒会通过 Telegram 通知你（请先绑定）。</li>
          </ul>
          <Typography.Title level={5}>AI 命令行工具</Typography.Title>
          <Typography.Paragraph>
            Droid、Claude Code、Codex 等命令行工具不走系统代理，开着 Clash 也会直连而登录失败。请在
            Clash 里打开 TUN 模式（虚拟网卡），它们就会走「🤖 AI 服务」。不想开
            TUN，也可以在终端里先设置{' '}
            <Typography.Text code>HTTPS_PROXY=http://127.0.0.1:7890</Typography.Text>
            （端口以 Clash 设置里的为准，Clash Verge 默认 7897）。
          </Typography.Paragraph>
        </Modal>
      )}
    </>
  );
}
