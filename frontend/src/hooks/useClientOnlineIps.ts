import { useState } from 'react';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { ClientOnlineIpsSchema, type ClientOnlineIps } from '@/generated/zod';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

// A client's IP slots in use, networks online and IP-limit bans, shared by every
// admin view that shows them. No email (add-client form) => every action no-ops.
export function useClientOnlineIps(email: string | undefined) {
  const [data, setData] = useState<ClientOnlineIps | null>(null);
  const [loadedAt, setLoadedAt] = useState(0);
  const [loading, setLoading] = useState(false);
  const [unbanning, setUnbanning] = useState<string | null>(null);

  async function load() {
    if (!email) return;
    setLoading(true);
    try {
      const msg = await HttpUtil.post(`/panel/api/clients/onlineIps/${encodeURIComponent(email)}`);
      const parsed = parseMsg(msg, ClientOnlineIpsSchema, 'clients/onlineIps');
      setData(parsed.success && parsed.obj ? parsed.obj : null);
      setLoadedAt(Date.now());
    } finally {
      setLoading(false);
    }
  }

  async function unban(network: string) {
    if (!email) return;
    setUnbanning(network);
    try {
      const msg = await HttpUtil.post(
        `/panel/api/clients/unbanIp/${encodeURIComponent(email)}`,
        { network },
        JSON_HEADERS,
      );
      if (msg?.success) await load();
    } finally {
      setUnbanning(null);
    }
  }

  function reset() {
    setData(null);
  }

  return { data, loadedAt, loading, unbanning, load, unban, reset };
}
