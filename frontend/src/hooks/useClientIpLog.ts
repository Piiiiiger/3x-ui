import { useState } from 'react';
import { HttpUtil } from '@/utils';
import {
  normalizeClientIpBans,
  normalizeClientIps,
  type ClientIpBan,
  type ClientIpInfo,
} from '@/lib/clients/ip-log';

interface ApiMsg<T = unknown> {
  success?: boolean;
  obj?: T;
}

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

// Fetch/mutate state for one client's IP log and IP-limit bans, shared by the
// edit form and the info card. No email (add-client form) => every action no-ops.
export function useClientIpLog(email: string | undefined) {
  const [ips, setIps] = useState<ClientIpInfo[]>([]);
  const [bans, setBans] = useState<ClientIpBan[]>([]);
  const [loadedAt, setLoadedAt] = useState(0);
  const [loading, setLoading] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [unbanning, setUnbanning] = useState<string | null>(null);

  async function load() {
    if (!email) return;
    setLoading(true);
    try {
      const path = encodeURIComponent(email);
      const [ipsMsg, bansMsg] = await Promise.all([
        HttpUtil.post(`/panel/api/clients/ips/${path}`) as Promise<ApiMsg<unknown[]>>,
        HttpUtil.post(`/panel/api/clients/ipBans/${path}`) as Promise<ApiMsg<unknown[]>>,
      ]);
      setIps(ipsMsg?.success ? normalizeClientIps(ipsMsg.obj) : []);
      setBans(bansMsg?.success ? normalizeClientIpBans(bansMsg.obj) : []);
      setLoadedAt(Date.now());
    } finally {
      setLoading(false);
    }
  }

  async function clear() {
    if (!email) return;
    setClearing(true);
    try {
      const msg = (await HttpUtil.post(
        `/panel/api/clients/clearIps/${encodeURIComponent(email)}`,
      )) as ApiMsg;
      if (msg?.success) setIps([]);
    } finally {
      setClearing(false);
    }
  }

  async function unban(network: string) {
    if (!email) return;
    setUnbanning(network);
    try {
      const msg = (await HttpUtil.post(
        `/panel/api/clients/unbanIp/${encodeURIComponent(email)}`,
        { network },
        JSON_HEADERS,
      )) as ApiMsg;
      if (msg?.success) await load();
    } finally {
      setUnbanning(null);
    }
  }

  function reset() {
    setIps([]);
    setBans([]);
  }

  return { ips, bans, loadedAt, loading, clearing, unbanning, load, clear, unban, reset };
}
