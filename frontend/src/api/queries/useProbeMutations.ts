import { useMutation, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';
import type { ProbeLinkInput, ProbeSettings } from '@/generated/zod';

// Both handlers bind JSON only; the default form encoding is refused.
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export function useProbeMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.probe.root() });

  const linksMut = useMutation({
    mutationFn: (links: ProbeLinkInput[]) =>
      HttpUtil.post('/panel/api/probe/links', { links }, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  const settingsMut = useMutation({
    mutationFn: (settings: ProbeSettings) =>
      HttpUtil.post('/panel/api/probe/settings', settings, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  return {
    saveLinks: (links: ProbeLinkInput[]) => linksMut.mutateAsync(links),
    saveSettings: (settings: ProbeSettings) => settingsMut.mutateAsync(settings),
  };
}
