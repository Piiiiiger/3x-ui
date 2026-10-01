import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { z } from 'zod';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { PlanSummarySchema, type PlanSummary } from '@/generated/zod';

const PlanListSchema = z
  .array(PlanSummarySchema)
  .nullable()
  .transform((value) => value ?? []);

async function fetchPlans(): Promise<PlanSummary[]> {
  const msg = await HttpUtil.get('/panel/api/plans/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch plans');
  return parseMsg(msg, PlanListSchema, 'plans/list').obj ?? [];
}

export function usePlansQuery() {
  const query = useQuery({ queryKey: keys.plans.list(), queryFn: fetchPlans });
  const plans = useMemo(() => query.data ?? [], [query.data]);
  return {
    plans,
    loading: query.isFetching,
    fetched: query.data !== undefined || query.isError,
    fetchError: query.error ? (query.error as Error).message : '',
    refetch: query.refetch,
  };
}
