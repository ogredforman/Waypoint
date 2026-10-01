// @vitest-environment jsdom
import { createRawSnippet, mount, unmount } from 'svelte'
import { describe, expect, it, vi } from 'vitest'
import Button from './Button.svelte'

const label = createRawSnippet(() => ({ render: () => '<span>Go</span>' }))

function render(props: Record<string, unknown>) {
  const target = document.body.appendChild(document.createElement('div'))
  const c = mount(Button, { target, props: { children: label, ...props } })
  const button = target.querySelector('button')!
  return {
    button,
    done: () => {
      unmount(c)
      target.remove()
    },
  }
}

describe('Button', () => {
  it('is a medium primary button of type button by default', () => {
    const { button, done } = render({})
    expect(button.type).toBe('button')
    expect(button.classList).toContain('shadow-glow')
    expect(button.classList).toContain('px-4')
    expect(button.textContent).toBe('Go')
    done()
  })

  it('styles each variant and size', () => {
    const cases: [Record<string, unknown>, string][] = [
      [{ variant: 'secondary' }, 'border'],
      [{ variant: 'quiet' }, 'bg-raised/50'],
      [{ variant: 'danger' }, 'bg-danger'],
      [{ size: 'sm' }, 'px-3'],
      [{ size: 'xs' }, 'text-xs'],
    ]
    for (const [props, cls] of cases) {
      const { button, done } = render(props)
      expect(button.classList).toContain(cls)
      done()
    }
  })

  it('red is only for danger', () => {
    for (const variant of ['primary', 'secondary', 'quiet']) {
      const { button, done } = render({ variant })
      expect(button.className).not.toMatch(/danger|red/)
      done()
    }
  })

  it('passes through type, disabled, extra classes, and clicks', () => {
    const onclick = vi.fn()
    const { button, done } = render({ type: 'submit', class: 'ml-1', onclick })
    expect(button.type).toBe('submit')
    expect(button.classList).toContain('ml-1')
    button.click()
    expect(onclick).toHaveBeenCalledOnce()
    done()
    const disabled = render({ disabled: true })
    expect(disabled.button.disabled).toBe(true)
    disabled.done()
  })
})
