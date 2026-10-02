import { useQuery, type QueryKey } from '@tanstack/react-query';

// For an answer that seeds a form. Each form that mounts reads what is stored
// now: what the cache holds from the form before it is never handed out.
export function useFormSeedQuery<T>(queryKey: QueryKey, queryFn: () => Promise<T>) {
  const query = useQuery({ queryKey, queryFn, staleTime: 0 });
  const answered = query.isFetchedAfterMount;
  return {
    data: answered ? query.data : undefined,
    fetchError: answered && query.error ? (query.error as Error).message : '',
  };
}
