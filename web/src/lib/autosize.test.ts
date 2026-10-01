// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { fitHeight } from './autosize'

// jsdom does no layout, so each test says how tall the content is.
function textarea(contentHeight: number, maxHeight = '256px'): HTMLTextAreaElement {
  const el = document.createElement('textarea')
  el.style.maxHeight = maxHeight
  el.style.borderTopWidth = '1px'
  el.style.borderBottomWidth = '1px'
  setContentHeight(el, contentHeight)
  document.body.append(el)
  return el
}

function setContentHeight(el: HTMLTextAreaElement, h: number) {
  Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => h })
}

afterEach(() => document.body.replaceChildren())

describe('fitHeight', () => {
  it('fits the content, borders included, without a scrollbar', () => {
    const el = textarea(100)
    fitHeight(el)
    expect(el.style.height).toBe('102px')
    expect(el.style.overflowY).toBe('hidden')
  })

  it('stops at the max height and scrolls past it', () => {
    const el = textarea(400)
    fitHeight(el)
    expect(el.style.height).toBe('256px')
    expect(el.style.overflowY).toBe('auto')
  })

  it('shrinks again when the text gets shorter', () => {
    const el = textarea(400)
    fitHeight(el)
    setContentHeight(el, 60)
    fitHeight(el)
    expect(el.style.height).toBe('62px')
    expect(el.style.overflowY).toBe('hidden')
  })

  it('grows freely when there is no max height', () => {
    const el = textarea(900, 'none')
    fitHeight(el)
    expect(el.style.height).toBe('902px')
    expect(el.style.overflowY).toBe('hidden')
  })

  it('keeps scroll containers where they were', () => {
    const box = document.createElement('div')
    document.body.append(box)
    const el = textarea(100)
    box.append(el)
    box.scrollTop = 40
    fitHeight(el)
    expect(box.scrollTop).toBe(40)
  })
})
