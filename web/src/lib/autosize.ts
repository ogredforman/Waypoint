// Text areas grow with their content: from their `rows` height up to their CSS max-height, and
// scroll only past it. There's no resize handle (Textarea sets resize-none).

// fitHeight sizes el to its content, capped by its computed max-height, and shows a scrollbar only
// when the content is taller than that cap.
export function fitHeight(el: HTMLTextAreaElement): void {
  // Collapsing the height for a moment can shorten a scroll container and move it; put it back.
  const scrolled: [Element, number][] = []
  for (let p = el.parentElement; p; p = p.parentElement) if (p.scrollTop > 0) scrolled.push([p, p.scrollTop])

  // With height auto the box falls back to its rows height, so scrollHeight is never less than it.
  // Height includes the border because Tailwind sets box-sizing: border-box.
  el.style.height = 'auto'
  el.style.overflowY = 'hidden'
  const style = getComputedStyle(el)
  const border = (parseFloat(style.borderTopWidth) || 0) + (parseFloat(style.borderBottomWidth) || 0)
  const needed = el.scrollHeight + border
  const max = parseFloat(style.maxHeight) // NaN for "none"
  const over = needed > max
  el.style.height = `${over ? max : needed}px`
  el.style.overflowY = over ? 'auto' : 'hidden'

  for (const [p, top] of scrolled) p.scrollTop = top
}

// refitOnResize is an attachment that refits el when its width changes (e.g. the diary panel is
// dragged, or a hidden text area is shown), since that changes how its text wraps.
export function refitOnResize(el: HTMLTextAreaElement): (() => void) | undefined {
  if (typeof ResizeObserver === 'undefined') return
  let width = el.clientWidth
  const observer = new ResizeObserver(() => {
    if (el.clientWidth === width) return // our own height change; nothing to do
    width = el.clientWidth
    fitHeight(el)
  })
  observer.observe(el)
  return () => observer.disconnect()
}
