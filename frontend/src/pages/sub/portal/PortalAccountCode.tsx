import { useState } from 'react';
import { Alert, Button, Modal, Space, Tag, Typography } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import type { PortalData } from '@/schemas/portal';

// An unbound account's link carries its code, so pressing Start in Telegram binds it.
function botLink(data: PortalData): string {
  const bot = `https://t.me/${encodeURIComponent(data.telegramBot)}`;
  return data.account && !data.telegramBound
    ? `${bot}?start=${encodeURIComponent(data.account.code)}`
    : bot;
}

export default function PortalAccountCode({
  data,
  label = '我的激活码',
}: {
  data: PortalData;
  label?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>{label}</Button>
      <Modal
        title="我的激活码与 Telegram"
        open={open}
        onCancel={() => setOpen(false)}
        footer={null}
      >
        <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
          <Typography.Text>账号：{data.email}</Typography.Text>
          {data.account ? (
            <Typography.Paragraph
              code
              copyable={{ text: data.account.code, icon: <CopyOutlined /> }}
              style={{ fontSize: 20, overflowWrap: 'anywhere', marginBottom: 0 }}
            >
              {data.account.code}
            </Typography.Paragraph>
          ) : (
            <Alert type="warning" title="暂时无法获取激活码，请刷新重试。" />
          )}
          <Tag color={data.telegramBound ? 'green' : 'default'}>
            {data.telegramBound ? 'Telegram 已绑定' : 'Telegram 未绑定'}
          </Tag>
          {data.telegramBot ? (
            <Button type="primary" href={botLink(data)} target="_blank" rel="noopener noreferrer">
              {data.telegramBound || !data.account ? '打开 Telegram 机器人' : '一键绑定 Telegram'}
            </Button>
          ) : (
            <Alert type="info" title="机器人尚未启用，管理员配置后即可绑定。" />
          )}
          <Typography.Paragraph>
            {data.telegramBound
              ? '此码固定对应你的账号，请勿转发给他人。续费请联系管理员。'
              : '点上面的按钮，在 Telegram 里按「开始」即可绑定；也可以把此码发给机器人。此码固定对应你的账号，请勿转发给他人。续费请联系管理员。'}
          </Typography.Paragraph>
          <Typography.Paragraph>
            日报默认北京时间 20:00 发送，包含剩余流量和使用情况。账号到期前 7、3、1
            天提醒，续费后按新日期计算；同时在线的 IP 超出上限时也会提醒。
          </Typography.Paragraph>
          <Typography.Text>
            在机器人里点按钮就能查看用量、在线设备和修改日报，也可以用命令：
          </Typography.Text>
          <div>
            <code>/start</code> 主菜单 · <code>/report</code> 用量 · <code>/ips</code> 在线设备 ·{' '}
            <code>/daily</code> 日报设置
          </div>
          {data.account && data.telegramBound && (
            <Typography.Text type="secondary">
              当前日报：{data.account.dailyEnabled ? '开启' : '关闭'} · {data.account.dailyTime}
              （北京时间）
            </Typography.Text>
          )}
        </Space>
      </Modal>
    </>
  );
}
