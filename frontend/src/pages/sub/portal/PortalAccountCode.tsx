import { useState } from 'react';
import { Alert, Button, Modal, Space, Tag, Typography } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import type { PortalData } from '@/schemas/portal';

export default function PortalAccountCode({ data }: { data: PortalData }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>我的激活码</Button>
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
            <Button
              type="primary"
              href={`https://t.me/${encodeURIComponent(data.telegramBot)}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              打开 Telegram 机器人
            </Button>
          ) : (
            <Alert type="info" title="机器人尚未启用，管理员配置后即可绑定。" />
          )}
          <Typography.Paragraph>
            此码固定对应你的账号。私聊机器人并发送此码即可绑定，请勿转发给他人。续费请联系管理员。
          </Typography.Paragraph>
          <Typography.Paragraph>
            日报默认北京时间 20:00 发送，包含剩余流量和使用情况。账号到期前 7、3、1
            天提醒，续费后按新日期计算。
          </Typography.Paragraph>
          <Typography.Text>机器人命令</Typography.Text>
          <div>
            <code>/report</code> 查询用量
          </div>
          <div>
            <code>/daily off</code> 关闭日报 · <code>/daily on</code> 开启日报
          </div>
          <div>
            <code>/daily 09:00</code> 改为北京时间每天 09:00
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
