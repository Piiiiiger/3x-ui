import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { ActivationCodeSchema, type ActivationCodeInput } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';

const BASE = '/panel/api/plans/codes';
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export function useActivationCodes(planId: number) {
  const queryClient = useQueryClient();
  const listKey = keys.activationCodes.list(planId);
  const query = useQuery({
    queryKey: listKey,
    queryFn: async () => {
      const reply = await HttpUtil.get(`${BASE}/${planId}`, undefined, { silent: true });
      if (!reply?.success) throw new Error(reply?.msg || 'Could not load activation codes');
      return z.array(ActivationCodeSchema).parse(reply.obj);
    },
  });
  const refresh = (reply: { success?: boolean } | null | undefined) => {
    if (reply?.success) void queryClient.invalidateQueries({ queryKey: listKey });
  };
  const create = useMutation({
    mutationFn: (input: ActivationCodeInput) => HttpUtil.post(`${BASE}/add`, input, JSON_HEADERS),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`${BASE}/del/${id}`),
    onSuccess: refresh,
  });
  return { ...query, create, remove };
}
