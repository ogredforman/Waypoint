// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProjectStatus } from '../lib/api/enums.gen'
import type { Project } from '../lib/api/types.gen'
import ProjectPicker from './ProjectPicker.svelte'

function project(id: number, title: string, status: ProjectStatus = 'active'): Project {
  return { id, title, why: '', definition_of_done: '', status, created_at: '', updated_at: '', last_activity_at: '' }
}

let target: HTMLElement
let component: ReturnType<typeof mount>
const onpick = vi.fn()

function render(projects: Project[], value: number | null) {
  component = mount(ProjectPicker, { target, props: { id: 'p', projects, value, onpick, missingLabel: 'This project' } })
  flushSync()
}

beforeEach(() => {
  onpick.mockClear()
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  unmount(component)
  target.remove()
})

const input = () => target.querySelector<HTMLInputElement>('[role=combobox]')!
const options = () => [...target.querySelectorAll('[role=option]')].map((o) => o.textContent?.trim())
const active = () => document.getElementById(input().getAttribute('aria-activedescendant') ?? '')?.textContent?.trim()

function key(k: string): KeyboardEvent {
  const e = new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true })
  input().dispatchEvent(e)
  flushSync()
  return e
}
function type(text: string) {
  input().value = text
  input().dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
}

const projects = [project(1, 'Garden shed'), project(2, 'Kiln'), project(3, 'Herb garden')]

describe('ProjectPicker', () => {
  it('shows the chosen project and opens on click with every choice', () => {
    render(projects, 2)
    expect(input().value).toBe('Kiln')
    expect(options()).toEqual([])
    input().click()
    flushSync()
    expect(input().getAttribute('aria-expanded')).toBe('true')
    expect(options()).toEqual(['No project', 'Garden shed', 'Kiln', 'Herb garden'])
    expect(active()).toBe('Kiln')
  })

  it('narrows as you type, and Enter picks the first match without submitting', () => {
    render(projects, null)
    type('garden')
    expect(options()).toEqual(['No project', 'Garden shed', 'Herb garden'])
    expect(key('ArrowDown').defaultPrevented).toBe(true)
    const enter = key('Enter')
    expect(enter.defaultPrevented).toBe(true)
    expect(onpick).toHaveBeenCalledWith(3)
    expect(options()).toEqual([])
  })

  it('says so when nothing matches, and no project stays a choice', () => {
    render(projects, 1)
    type('zzz')
    expect(options()).toEqual(['No project'])
    expect(target.textContent).toContain('No projects match.')
    key('ArrowUp')
    key('Enter')
    expect(onpick).toHaveBeenCalledWith(null)
  })

  it('arrow keys wrap around', () => {
    render(projects, null)
    key('ArrowDown')
    expect(active()).toBe('No project')
    key('ArrowUp')
    expect(active()).toBe('Herb garden')
    key('ArrowDown')
    expect(active()).toBe('No project')
  })

  it('Escape closes without picking and puts the label back', () => {
    render(projects, 2)
    type('gar')
    const esc = key('Escape')
    expect(esc.defaultPrevented).toBe(true)
    expect(options()).toEqual([])
    expect(input().value).toBe('Kiln')
    expect(onpick).not.toHaveBeenCalled()
  })

  it('picks an option with the mouse', () => {
    render(projects, null)
    input().click()
    flushSync()
    ;[...target.querySelectorAll<HTMLElement>('[role=option]')].find((o) => o.textContent?.trim() === 'Kiln')!.click()
    flushSync()
    expect(onpick).toHaveBeenCalledWith(2)
  })

  it('shows an archived project when it is the one chosen', () => {
    render([project(1, 'Garden shed'), project(4, 'Old kiln', 'archived')], 4)
    expect(input().value).toBe('Old kiln')
  })

  it('names a chosen project the list lacks with the missing label', () => {
    render([], 5)
    expect(input().value).toBe('This project')
    input().click()
    flushSync()
    expect(options()).toEqual(['No project', 'This project'])
  })
})
