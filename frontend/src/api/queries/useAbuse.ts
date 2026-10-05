import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseRequired } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import {
  AbuseHistorySchema,
  AbuseOverviewSchema,
  type AbuseHistory,
  type AbuseOverview,
  type AbuseSettings,
} from '@/generated/zod';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export type AbuseMode = 'off' | 'observe' | 'enforce';

async function fetchOverview(): Promise<AbuseOverview> {
  const msg = await HttpUtil.get('/panel/api/abuse/overview', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || '读取防滥用设置失败');
  return parseRequired(msg, AbuseOverviewSchema, 'abuse/overview');
}

export function useAbuseOverview() {
  return useQuery({
    queryKey: keys.abuse.overview(),
    queryFn: fetchOverview,
    refetchInterval: 15000,
    refetchIntervalInBackground: false,
  });
}

async function fetchHistory(email: string): Promise<AbuseHistory> {
  const msg = await HttpUtil.get(
    `/panel/api/abuse/history/${encodeURIComponent(email)}`,
    undefined,
    {
      silent: true,
    },
  );
  if (!msg?.success) throw new Error(msg?.msg || '读取封禁记录失败');
  return parseRequired(msg, AbuseHistorySchema, 'abuse/history');
}

export function useAbuseHistory(email: string, enabled = true) {
  return useQuery({
    queryKey: keys.abuse.history(email),
    queryFn: () => fetchHistory(email),
    enabled: enabled && email !== '',
  });
}

export function useAbuseMutations() {
  const queryClient = useQueryClient();
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.abuse.root() });
  const done = (msg: { success?: boolean } | undefined) => {
    if (msg?.success) void refresh();
    return msg;
  };
  const mode = useMutation({
    mutationFn: ({ nodeId, mode }: { nodeId: number; mode: AbuseMode }) =>
      HttpUtil.post('/panel/api/abuse/mode', { nodeId, mode }, JSON_HEADERS),
    onSuccess: done,
  });
  const settings = useMutation({
    mutationFn: (next: AbuseSettings) =>
      HttpUtil.post('/panel/api/abuse/settings', next, JSON_HEADERS),
    onSuccess: done,
  });
  const lift = useMutation({
    mutationFn: (email: string) =>
      HttpUtil.post(`/panel/api/abuse/lift/${encodeURIComponent(email)}`, {}, JSON_HEADERS),
    onSuccess: done,
  });
  const forgive = useMutation({
    mutationFn: (email: string) =>
      HttpUtil.post(`/panel/api/abuse/forgive/${encodeURIComponent(email)}`, {}, JSON_HEADERS),
    onSuccess: done,
  });
  return {
    setMode: (nodeId: number, next: AbuseMode) => mode.mutateAsync({ nodeId, mode: next }),
    saveSettings: (next: AbuseSettings) => settings.mutateAsync(next),
    lift: (email: string) => lift.mutateAsync(email),
    forgive: (email: string) => forgive.mutateAsync(email),
  };
}
