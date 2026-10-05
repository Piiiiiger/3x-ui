import { useState } from 'react';
import { Alert, Button, Modal, Spin } from 'antd';
import { HistoryOutlined } from '@ant-design/icons';

import { useAbuseHistory, useAbuseMutations } from '@/api/queries/useAbuse';
import { BanHistory } from './BanHistory';

// The admin's view of one account's bans, read when opened, with the actions
// that end a ban or clear the strikes.
export function ClientBanHistoryButton({ email }: { email: string }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const { data, error, dataUpdatedAt } = useAbuseHistory(email, open);
  const { lift, forgive } = useAbuseMutations();

  async function run(action: (email: string) => Promise<unknown>) {
    setBusy(true);
    try {
      await action(email);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Button size="small" icon={<HistoryOutlined />} onClick={() => setOpen(true)}>
        查看
      </Button>
      {open && (
        <Modal open title={`${email} 的封禁记录`} footer={null} onCancel={() => setOpen(false)}>
          {error ? (
            <Alert type="error" showIcon title={(error as Error).message} />
          ) : !data ? (
            <Spin />
          ) : (
            <BanHistory
              history={data}
              nowMs={dataUpdatedAt}
              busy={busy}
              onLift={() => void run(lift)}
              onForgive={() => void run(forgive)}
            />
          )}
        </Modal>
      )}
    </>
  );
}
