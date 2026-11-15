<script lang="ts">
  // Pagination for a paged list: the page label on the left, the prev/next
  // pair on the right. Both lists on the jailbreak page used to carry this
  // markup twice, so a change to the button treatment had to be made twice.
  interface Props {
    page: number
    pageCount: number
    total: number
    noun?: string
    onPage: (page: number) => void
  }
  let { page, pageCount, total, noun = 'preset', onPage }: Props = $props()
</script>

<div class="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-border-subtle">
  <span class="text-xs text-text-muted">
    Page {page} of {pageCount} · {total} {noun}{total === 1 ? '' : 's'}
  </span>
  <div class="flex items-center gap-2">
    <button
      type="button"
      onclick={() => onPage(Math.max(1, page - 1))}
      disabled={page <= 1}
      aria-label="Previous page"
      class="inline-flex items-center justify-center min-h-9 min-w-9 px-3 py-1.5 rounded-[8px] bg-surface-3 border border-border-subtle text-text-main text-sm font-medium cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed hover:bg-brand-500 hover:text-white transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
    >
      ‹
    </button>
    <button
      type="button"
      onclick={() => onPage(Math.min(pageCount, page + 1))}
      disabled={page >= pageCount}
      aria-label="Next page"
      class="inline-flex items-center justify-center min-h-9 min-w-9 px-3 py-1.5 rounded-[8px] bg-surface-3 border border-border-subtle text-text-main text-sm font-medium cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed hover:bg-brand-500 hover:text-white transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
    >
      ›
    </button>
  </div>
</div>
