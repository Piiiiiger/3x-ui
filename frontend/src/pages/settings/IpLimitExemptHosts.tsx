import { useEffect, useState } from 'react';
import { Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

import { HttpUtil } from '@/utils';

interface ApiMsg<T = unknown> {
  success?: boolean;
  obj?: T;
}

type ExemptHost = { name: string; addresses: string[] };

function normalizeExemptHosts(obj: unknown): ExemptHost[] {
  if (!Array.isArray(obj)) return [];
  const out: ExemptHost[] = [];
  for (const x of obj) {
    if (!x || typeof x !== 'object') continue;
    const o = x as Record<string, unknown>;
    const addresses = Array.isArray(o.addresses)
      ? o.addresses.filter((a): a is string => typeof a === 'string' && a !== '')
      : [];
    if (addresses.length > 0)
      out.push({ name: typeof o.name === 'string' ? o.name : '', addresses });
  }
  return out;
}

// Read-only: the panel derives this list from the servers it knows, so a new
// relay is covered the moment it is added.
export default function IpLimitExemptHosts() {
  const { t } = useTranslation();
  const [hosts, setHosts] = useState<ExemptHost[]>([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const msg = (await HttpUtil.post('/panel/api/setting/ipLimitExempt', undefined, {
        silent: true,
      })) as ApiMsg<unknown>;
      if (!cancelled) setHosts(msg?.success ? normalizeExemptHosts(msg.obj) : []);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {hosts.map((host) => (
        <div key={host.name || '\u0000self'} style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          <Typography.Text strong style={{ marginInlineEnd: 4 }}>
            {host.name || t('pages.clients.ipExemptThisPanel')}
          </Typography.Text>
          {host.addresses.map((address) => (
            <Tag key={address} style={{ margin: 0, fontFamily: 'ui-monospace, monospace' }}>
              {address}
            </Tag>
          ))}
        </div>
      ))}
    </div>
  );
}
