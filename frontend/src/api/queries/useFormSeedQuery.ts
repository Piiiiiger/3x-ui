import { useState } from 'react';
import { useQuery, type QueryKey } from '@tanstack/react-query';

// For the answer a form is built from: the first one after it mounts, never
// what the cache holds, and kept whatever the query fetches or fails at later.
export function useFormSeedQuery<T>(queryKey: QueryKey, queryFn: () => Promise<T>) {
  const query = useQuery({ queryKey, queryFn, staleTime: 0 });
  const [seed, setSeed] = useState<{ data: T } | null>(null);
  if (seed) return { data: seed.data, fetchError: '' };
  if (query.isFetchedAfterMount && query.isSuccess) {
    setSeed({ data: query.data });
    return { data: query.data, fetchError: '' };
  }
  const failed = query.isFetchedAfterMount && query.error;
  return { data: undefined, fetchError: failed ? (query.error as Error).message : '' };
}
