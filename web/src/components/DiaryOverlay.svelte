<script lang="ts">
  import Button from './ui/Button.svelte'
  import BookOpen from '@lucide/svelte/icons/book-open'
  import DiaryContent from './DiaryContent.svelte'

  // The diary on narrow windows (below lg): a floating button that opens a full-height overlay
  // (ADR 0004). App mounts it only below lg. A modal <dialog> gives Esc to close and keeps focus
  // inside while open. The button shows no count or badge; pending items shouldn't nag (SPEC §5).
  // The diary content is mounted only while the overlay is open, so there's never a second copy.

  let dialog: HTMLDialogElement
  let open = $state(false)

  function show() {
    open = true
    dialog.showModal()
  }
</script>

<button
  type="button"
  class="fixed right-5 bottom-5 z-20 flex items-center gap-2 rounded-full bg-linear-to-b from-accent-strong to-accent-deep px-5 py-3 text-sm font-medium text-on-accent shadow-glow hover:brightness-110"
  aria-haspopup="dialog"
  onclick={show}
>
  <BookOpen size={18} aria-hidden="true" />
  Diary
</button>

<dialog
  bind:this={dialog}
  aria-labelledby="diary-overlay-title"
  class="fixed inset-y-0 right-0 left-auto m-0 h-dvh max-h-none w-full max-w-md border-l border-line bg-surface p-0 text-ink shadow-edge backdrop:bg-canvas/70"
  onclose={() => (open = false)}
>
  <div class="flex h-full flex-col">
    <div class="flex items-center justify-between gap-2 border-b border-line px-gutter py-3">
      <h2 id="diary-overlay-title" class="font-semibold tracking-tight">Diary</h2>
      <Button variant="quiet" size="xs" onclick={() => dialog.close()}>Close</Button>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto p-gutter">
      {#if open}<DiaryContent />{/if}
    </div>
  </div>
</dialog>
