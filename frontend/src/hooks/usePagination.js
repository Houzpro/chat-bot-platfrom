import { useCallback, useState } from 'react'

// usePagination — small state holder for paginated lists.
// Keeps page/limit and a meta snapshot returned from the API so components
// don't reinvent this glue. Pass defaults like { page: 1, limit: 20 }.
export function usePagination(defaults = {}) {
  const [page, setPage] = useState(defaults.page ?? 1)
  const [limit, setLimit] = useState(defaults.limit ?? 20)
  const [meta, setMeta] = useState(null)

  // Reset to page 1 — use when filters/search change underneath so we don't
  // end up on a page that no longer exists.
  const reset = useCallback(() => setPage(1), [])

  return { page, setPage, limit, setLimit, meta, setMeta, reset }
}
