import { keepPreviousData, useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { AiUsageOverviewSchema, type AiUsageOverview } from '@/generated/zod';

export type AiUsagePeriod = AiUsageOverview['period'];
export type AiUsageApp = 'all' | 'claude' | 'codex';

const REFRESH_MS = 60_000;

async function fetchAiUsageOverview(
  period: AiUsagePeriod,
  deviceId: number,
  app: AiUsageApp,
): Promise<AiUsageOverview | null> {
  const msg = await HttpUtil.get(
    '/panel/api/aiUsage/overview',
    { period, deviceId, app },
    { silent: true },
  );
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the AI usage overview');
  return parseMsg(msg, AiUsageOverviewSchema, 'aiUsage/overview').obj ?? null;
}

/** The AI usage page for one period and filter; switching keeps the last answer up meanwhile. */
export function useAiUsageOverviewQuery(period: AiUsagePeriod, deviceId: number, app: AiUsageApp) {
  const query = useQuery({
    queryKey: keys.aiUsage.overview(period, deviceId, app),
    queryFn: () => fetchAiUsageOverview(period, deviceId, app),
    refetchInterval: REFRESH_MS,
    placeholderData: keepPreviousData,
  });
  return {
    overview: query.data ?? null,
    fetched: query.data !== undefined || query.isError,
    fetchError: query.error ? (query.error as Error).message : '',
    refetch: query.refetch,
  };
}
