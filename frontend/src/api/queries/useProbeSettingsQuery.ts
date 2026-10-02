import { HttpUtil } from '@/utils';
import { parseRequired } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { ProbeSettingsSchema, type ProbeSettings } from '@/generated/zod';
import { useFormSeedQuery } from './useFormSeedQuery';

async function fetchProbeSettings(): Promise<ProbeSettings> {
  const msg = await HttpUtil.get('/panel/api/probe/settings', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the probe settings');
  return parseRequired(msg, ProbeSettingsSchema, 'probe/settings');
}

export function useProbeSettingsQuery() {
  const { data, fetchError } = useFormSeedQuery(keys.probe.settings(), fetchProbeSettings);
  return { settings: data, fetchError };
}
