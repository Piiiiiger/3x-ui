import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';
import { HttpUtil } from '@/utils';
import { parseRequired } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';

const PlanNodeOptionSchema = z.object({
  key: z.string(),
  label: z.string(),
  inboundId: z.number(),
  relayInboundId: z.number(),
});
export type PlanNodeOption = z.infer<typeof PlanNodeOptionSchema>;

export function usePlanNodeOptions() {
  return useQuery({
    queryKey: [...keys.plans.root(), 'nodeOptions'],
    queryFn: async () => {
      const msg = await HttpUtil.get('/panel/api/plans/nodeOptions', undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || '无法加载直连和中转节点');
      return parseRequired(msg, z.array(PlanNodeOptionSchema), 'plans/nodeOptions');
    },
  });
}
