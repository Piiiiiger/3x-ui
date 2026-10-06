import { useState } from 'react';
import { Button, Modal, Typography } from 'antd';
import {
  AndroidFilled,
  AppleFilled,
  CheckCircleFilled,
  ClockCircleOutlined,
  CodeOutlined,
  DashboardOutlined,
  EnvironmentOutlined,
  FileSearchOutlined,
  LockOutlined,
  MailOutlined,
  ReadOutlined,
  RightOutlined,
  SecurityScanOutlined,
  StopOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  UserAddOutlined,
  WarningOutlined,
} from '@ant-design/icons';

import './PortalRules.css';

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

// Short enough for one line in two columns on a phone.
const FORBIDDEN = [
  { icon: <UserAddOutlined />, text: '批量注册账号' },
  { icon: <FileSearchOutlined />, text: '爬虫抓取网页' },
  { icon: <DashboardOutlined />, text: '长时间持续满速' },
  { icon: <ThunderboltOutlined />, text: '无故反复测速' },
  { icon: <SecurityScanOutlined />, text: '端口扫描、攻击' },
  { icon: <MailOutlined />, text: '垃圾邮件、BT 下载' },
];

function SettingsPath({ steps }: { steps: string[] }) {
  return (
    <span className="portal-rules-path">
      {steps.map((step, i) => (
        <span key={step}>
          {i > 0 && <RightOutlined className="portal-rules-path-sep" />}
          <kbd>{step}</kbd>
        </span>
      ))}
    </span>
  );
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
          width={640}
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
          <div className="portal-rules">
            <section className="portal-rules-alert" role="alert">
              <div className="portal-rules-alert-head">
                <span className="portal-rules-alert-icon">
                  <EnvironmentOutlined />
                </span>
                <div>
                  <div className="portal-rules-alert-title">
                    手机请务必关闭定位服务，防止节点「送中」
                  </div>
                  <p className="portal-rules-alert-text">
                    开着定位时，Google 会读到手机的真实位置，把你正在用的节点 IP
                    判定为中国大陆，也就是「送中」；代理改变不了定位。节点被送中后，所有人用它访问
                    Google、YouTube、Gemini 都会受影响。
                  </p>
                </div>
              </div>
              <div className="portal-rules-steps">
                <div className="portal-rules-step">
                  <span className="portal-rules-step-os">
                    <AppleFilled /> iPhone
                  </span>
                  <SettingsPath steps={['设置', '隐私与安全性', '定位服务', '关闭']} />
                </div>
                <div className="portal-rules-step">
                  <span className="portal-rules-step-os">
                    <AndroidFilled /> 安卓
                  </span>
                  <SettingsPath steps={['下拉快捷开关', '关闭「位置信息」']} />
                </div>
              </div>
            </section>

            <section className="portal-rules-section">
              <h3 className="portal-rules-heading">
                <StopOutlined /> 禁止以下行为
                <span className="portal-rules-badge">系统自动检测</span>
              </h3>
              <ul className="portal-rules-grid">
                {FORBIDDEN.map((item) => (
                  <li key={item.text}>
                    {item.icon}
                    <span>{item.text}</span>
                  </li>
                ))}
                <li className="is-wide">
                  <TeamOutlined />
                  <span>{sharing}</span>
                </li>
              </ul>
              <p className="portal-rules-ok">
                <CheckCircleFilled />
                <span>
                  正常下载软件、系统更新不受影响：连续满速 2 小时只会提醒，持续 4 小时才封禁。
                </span>
              </p>
            </section>

            <section className="portal-rules-section">
              <h3 className="portal-rules-heading">
                <WarningOutlined /> 处罚
              </h3>
              <div className="portal-rules-penalties">
                <div className="portal-rules-penalty">
                  <ClockCircleOutlined />
                  <strong>封禁 30 分钟</strong>
                  <span>每次违规都会封禁，到时自动恢复</span>
                </div>
                <div className="portal-rules-penalty">
                  <LockOutlined />
                  <strong>第 4 次停用</strong>
                  <span>30 天内违规超过 3 次，账号停用，需联系管理员说明情况</span>
                </div>
              </div>
              <ul className="portal-rules-notes">
                <li>同时在线 IP 超出上限只会暂停超出的 IP 30 分钟，不计入违规次数。</li>
                <li>封禁和提醒会通过 Telegram 通知你（请先绑定）。</li>
              </ul>
            </section>

            <section className="portal-rules-section">
              <h3 className="portal-rules-heading">
                <CodeOutlined /> AI 命令行工具
              </h3>
              <p className="portal-rules-text">
                Droid、Claude Code、Codex 等命令行工具不走系统代理，开着 Clash
                也会直连而登录失败。请在 Clash 里打开
                <strong> TUN 模式</strong>（虚拟网卡），它们就会走「🤖 AI 服务」。
              </p>
              <div className="portal-rules-command">
                <span className="portal-rules-command-label">不想开 TUN，先在终端设置：</span>
                <Typography.Text code copyable>
                  HTTPS_PROXY=http://127.0.0.1:7890
                </Typography.Text>
              </div>
              <p className="portal-rules-muted">
                端口以 Clash 设置里的为准，Clash Verge 默认 7897。
              </p>
            </section>
          </div>
        </Modal>
      )}
    </>
  );
}
