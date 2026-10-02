import { useState } from 'react';

// Counts how often a modal has opened. As a key it gives every opening a form
// of its own, with neither the edits nor the data of the opening before it.
export function useOpenings(open: boolean): number {
  const [last, setLast] = useState({ open, count: 0 });
  if (last.open === open) return last.count;
  const next = { open, count: open ? last.count + 1 : last.count };
  setLast(next);
  return next.count;
}
