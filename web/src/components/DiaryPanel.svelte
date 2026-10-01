<script lang="ts">
  import BookOpen from '@lucide/svelte/icons/book-open'
  import {
    browserStorage,
    loadPrefs,
    MAX_WIDTH,
    MIN_WIDTH,
    savePrefs,
    widthForDrag,
    widthForKey,
  } from '../lib/diaryPanel'
  import { paths } from '../lib/router'
  import DiaryContent from './DiaryContent.svelte'

  // The diary beside the main pane on wide windows (ADR 0004). App mounts it only from lg up;
  // DiaryOverlay takes over below that.

  const storage = browserStorage()
  const initial = loadPrefs(storage)
  let width = $state(initial.width)
  let open = $state(initial.open)
  let drag = $state<{ startX: number; startWidth: number } | null>(null)

  // Save once a drag ends rather than on every pointer move.
  $effect(() => {
    if (!drag) savePrefs(storage, { width, open })
  })

  $effect(() => {
    document.documentElement.classList.toggle('resizing', drag !== null)
  })

  function startDrag(e: PointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    drag = { startX: e.clientX, startWidth: width }
  }

  function moveDrag(e: PointerEvent) {
    if (drag) width = widthForDrag(drag.startWidth, drag.startX, e.clientX)
  }

  function endDrag() {
    drag = null
  }

  function resizeByKey(e: KeyboardEvent) {
    const next = widthForKey(width, e.key, e.shiftKey)
    if (next === null) return
    e.preventDefault()
    width = next
  }
</script>

{#if open}
  <aside
    id="diary-panel"
    aria-labelledby="diary-panel-title"
    class="relative flex shrink-0 flex-col border-l border-line bg-surface/95 shadow-edge "
    style:width="{width}px"
  >
    <!-- A focusable separator is an interactive widget (a window splitter) in WAI-ARIA. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize the diary"
      aria-controls="diary-panel"
      aria-valuenow={width}
      aria-valuemin={MIN_WIDTH}
      aria-valuemax={MAX_WIDTH}
      tabindex="0"
      class="group absolute inset-y-0 -left-1.5 z-10 flex w-3 cursor-col-resize justify-center focus-visible:outline-none"
      onpointerdown={startDrag}
      onpointermove={moveDrag}
      onpointerup={endDrag}
      onpointercancel={endDrag}
      onlostpointercapture={endDrag}
      onkeydown={resizeByKey}
    >
      <span
        class={[
          'h-full w-px transition group-hover:w-0.5 group-hover:bg-accent/70 group-focus-visible:w-0.5 group-focus-visible:bg-accent',
          drag ? 'w-0.5 bg-accent' : 'bg-transparent',
        ]}
      ></span>
    </div>

    <div class="flex items-center justify-between gap-2 border-b border-line px-gutter py-3">
      <h2 id="diary-panel-title" class="font-semibold tracking-tight">Diary</h2>
      <div class="flex items-center gap-1 text-label">
        <a href={paths.diary} class="rounded-control px-2 py-1 text-ink-muted hover:text-ink">Full page</a>
        <button
          type="button"
          class="rounded-control px-2 py-1 text-ink-muted hover:text-ink"
          aria-expanded="true"
          aria-controls="diary-panel"
          onclick={() => (open = false)}
        >
          Hide
        </button>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto p-gutter">
      <DiaryContent />
    </div>
  </aside>
{:else}
  <div class="flex shrink-0 flex-col items-center border-l border-line bg-surface/80 px-1.5 py-4">
    <!-- The label names the button; the tooltip shows it on hover and keyboard focus. -->
    <button
      type="button"
      class="group relative rounded-control p-2 text-ink-muted hover:text-accent focus-visible:text-accent"
      aria-label="Open the diary"
      aria-expanded="false"
      onclick={() => (open = true)}
    >
      <BookOpen size={20} aria-hidden="true" />
      <span
        aria-hidden="true"
        class="pointer-events-none absolute top-1/2 right-full mr-2 -translate-y-1/2 rounded-control border border-line bg-surface px-2 py-1 text-label whitespace-nowrap text-ink opacity-0 shadow-edge transition group-hover:opacity-100 group-focus-visible:opacity-100"
      >
        Open the diary
      </span>
    </button>
  </div>
{/if}
