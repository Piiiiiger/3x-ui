import { z } from 'zod';

import { HttpUtil } from '@/utils';
import { parseRequired } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { ProbeLinkViewSchema, type ProbeLinkView } from '@/generated/zod';
import { useFormSeedQuery } from './useFormSeedQuery';

const ProbeLinkListSchema = z.array(ProbeLinkViewSchema);

async function fetchProbeLinks(): Promise<ProbeLinkView[]> {
  const msg = await HttpUtil.get('/panel/api/probe/links', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the probe links');
  return parseRequired(msg, ProbeLinkListSchema, 'probe/links');
}

export function useProbeLinksQuery() {
  const { data, fetchError } = useFormSeedQuery(keys.probe.links(), fetchProbeLinks);
  return { links: data, fetchError };
}
