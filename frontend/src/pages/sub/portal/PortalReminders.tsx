import { useEffect, useState } from 'react';
import { Button, Tag } from 'antd';
import { ClockCircleOutlined, SendOutlined } from '@ant-design/icons';
import type { PortalData } from '@/schemas/portal';
import PortalAccountCode from './PortalAccountCode';

export default function PortalReminders({ data }: { data: PortalData }) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(timer);
  }, []);
  const expiry = Number(data.page?.expire ?? 0) * 1000;
  const known = !!data.page && Number.isFinite(expiry);
  const dated = known && expiry > 0;
  const expired = dated && expiry <= now;
  const days = dated ? Math.ceil((expiry - now) / 86_400_000) : 0;
  const urgent = dated && days <= 7;
  const expiryText = !known
    ? '暂时无法获取'
    : expiry < 0
      ? '首次使用后开始计时'
      : expiry === 0
        ? '长期有效'
        : new Intl.DateTimeFormat('zh-CN', {
            timeZone: 'Asia/Shanghai',
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            hour12: false,
          }).format(expiry);
  return (
    <section className="portal-reminders" aria-label="到期时间与 Telegram 提醒">
      <div className={`portal-reminder portal-expiry${urgent ? ' is-urgent' : ''}`}>
        <div className="portal-reminder-heading">
          <ClockCircleOutlined /> 账号到期时间
          {dated && (
            <Tag color={expired ? 'red' : urgent ? 'orange' : 'blue'}>
              {expired ? '已到期' : `剩余 ${days} 天`}
            </Tag>
          )}
        </div>
        <strong className="portal-expiry-date">{expiryText}</strong>
        <p>
          {dated ? '北京时间 · ' : ''}
          {expired ? '请联系管理员续费，恢复使用。' : '请留意到期时间，续费请联系管理员。'}
        </p>
      </div>
      <div className="portal-reminder portal-telegram">
        <div className="portal-reminder-heading">
          <SendOutlined /> Telegram 日报与到期提醒
          <Tag color={data.telegramBound ? 'green' : 'orange'}>
            {data.telegramBound ? '已绑定' : '未绑定'}
          </Tag>
        </div>
        <p>
          {data.telegramBound
            ? '到期前 7、3、1 天提醒，续费后按新日期计算。'
            : '绑定机器人，接收流量日报和到期前 7、3、1 天提醒。'}
        </p>
        <p className="portal-reminder-detail">
          {data.telegramBound && data.account
            ? `日报${data.account.dailyEnabled ? '已开启' : '已关闭'} · ${data.account.dailyTime}（北京时间）`
            : '日报默认每天 20:00（北京时间），可关闭或修改时间。'}
        </p>
        <div className="portal-reminder-actions">
          {data.telegramBot && (
            <Button
              type="primary"
              icon={<SendOutlined />}
              href={`https://t.me/${encodeURIComponent(data.telegramBot)}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              打开机器人
            </Button>
          )}
          <PortalAccountCode
            data={data}
            label={data.telegramBound ? '查看绑定与设置' : '获取激活码并绑定'}
          />
        </div>
      </div>
    </section>
  );
}
