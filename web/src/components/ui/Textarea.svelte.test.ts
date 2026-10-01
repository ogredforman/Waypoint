// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import Textarea from './Textarea.svelte'

// jsdom does no layout: make the content 20px per line.
const descriptor = Object.getOwnPropertyDescriptor(Element.prototype, 'scrollHeight')
beforeAll(() => {
  Object.defineProperty(HTMLTextAreaElement.prototype, 'scrollHeight', {
    configurable: true,
    get(this: HTMLTextAreaElement) {
      return this.value.split('\n').length * 20
    },
  })
})
afterAll(() => {
  delete (HTMLTextAreaElement.prototype as { scrollHeight?: number }).scrollHeight
  if (descriptor) Object.defineProperty(Element.prototype, 'scrollHeight', descriptor)
})

describe('Textarea', () => {
  it('has no resize handle and a max height', () => {
    const target = document.body.appendChild(document.createElement('div'))
    const c = mount(Textarea, { target, props: { id: 't' } })
    const plain = target.querySelector('textarea')!
    expect(plain.classList).toContain('resize-none')
    expect(plain.classList).toContain('max-h-64')
    unmount(c)
    const tall = mount(Textarea, { target, props: { id: 't', tall: true } })
    expect(target.querySelector('textarea')!.classList).toContain('max-h-96')
    unmount(tall)
    target.remove()
  })

  it('grows as text is typed and fits a value set from outside', () => {
    const target = document.body.appendChild(document.createElement('div'))
    const props = $state({ id: 't', value: 'one' })
    const c = mount(Textarea, { target, props })
    flushSync()
    const el = target.querySelector('textarea')!
    expect(el.style.height).toBe('20px')

    el.value = 'one\ntwo\nthree'
    el.dispatchEvent(new Event('input', { bubbles: true }))
    flushSync()
    expect(el.style.height).toBe('60px')

    props.value = ''
    flushSync()
    expect(el.style.height).toBe('20px')
    unmount(c)
    target.remove()
  })
})
