import React from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import './Pagination.css'

// buildPageList — returns the sequence of pages + ellipsis tokens to render.
// Shows first/last always, the current page with a neighbour on each side,
// and the string '…' where a gap is collapsed. Keeps the control compact
// even with hundreds of pages.
function buildPageList(current, totalPages) {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1)
  }

  const pages = new Set([1, totalPages, current, current - 1, current + 1])
  const sorted = [...pages].filter(n => n >= 1 && n <= totalPages).sort((a, b) => a - b)

  const out = []
  let prev = 0
  for (const n of sorted) {
    if (prev && n - prev > 1) out.push('…')
    out.push(n)
    prev = n
  }
  return out
}

function Pagination({ meta, onPageChange, className = '' }) {
  if (!meta || meta.total_pages <= 1) return null

  const { page, total_pages: totalPages, total, has_prev, has_next } = meta
  const items = buildPageList(page, totalPages)

  return (
    <nav className={`pagination ${className}`} aria-label="Pagination">
      <button
        type="button"
        className="pagination-btn"
        onClick={() => onPageChange(page - 1)}
        disabled={!has_prev}
        aria-label="Previous page"
      >
        <ChevronLeft size={16} />
      </button>

      {items.map((item, idx) => (
        item === '…' ? (
          <span key={`gap-${idx}`} className="pagination-gap">…</span>
        ) : (
          <button
            key={item}
            type="button"
            className={`pagination-btn${item === page ? ' active' : ''}`}
            onClick={() => onPageChange(item)}
            aria-current={item === page ? 'page' : undefined}
            aria-label={`Page ${item}`}
          >
            {item}
          </button>
        )
      ))}

      <button
        type="button"
        className="pagination-btn"
        onClick={() => onPageChange(page + 1)}
        disabled={!has_next}
        aria-label="Next page"
      >
        <ChevronRight size={16} />
      </button>

      <span className="pagination-summary">{total} total</span>
    </nav>
  )
}

export default Pagination
