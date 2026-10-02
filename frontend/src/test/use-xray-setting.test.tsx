import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useXraySetting } from '@/hooks/useXraySetting';
import { makeTestQueryClient } from '@/test/test-utils';
import { HttpUtil, Msg } from '@/utils';

function xrayPayload(overrides: Record<string, unknown> = {}) {
  return {
    xraySetting: {},
    ...overrides,
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', []));
});

describe('useXraySetting', () => {
  it('keeps local edits when the config refetches while the editor is dirty', async () => {
    let payload = xrayPayload({ xraySetting: { log: { loglevel: 'warning' } } });
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url) => {
      if (url === '/panel/api/xray/') return new Msg(true, '', JSON.stringify(payload));
      return new Msg(true, '');
    });
    const queryClient = makeTestQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useXraySetting(), { wrapper });

    await waitFor(() => expect(result.current.fetched).toBe(true));
    act(() => result.current.setXraySetting('{"outbounds":[]}'));
    payload = xrayPayload({ xraySetting: { log: { loglevel: 'debug' } } });
    await act(async () => result.current.fetchAll());

    await waitFor(() => expect(queryClient.isFetching()).toBe(0));
    expect(result.current.xraySetting).toBe('{"outbounds":[]}');
    expect(result.current.saveDisabled).toBe(false);
  });
});
