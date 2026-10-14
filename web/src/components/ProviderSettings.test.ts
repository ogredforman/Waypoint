// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useProfile } from '../lib/api/client'
import type { Provider } from '../lib/api/types.gen'
import ProviderSettings from './ProviderSettings.svelte'

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let current: Provider
let calls: { method: string; url: string; body?: unknown }[]
let reply: Response | null

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  const body = init?.body ? JSON.parse(init.body as string) : undefined
  calls.push({ method, url, body })
  if (reply) return reply
  if (method === 'PUT') {
    current = {
      ...current,
      provider: body.provider,
      model: body.model,
      endpoint: body.endpoint,
      api_key_set: body.api_key === undefined ? current.api_key_set : body.api_key !== '',
    }
  }
  return json(current)
})

beforeEach(() => {
  calls = []
  reply = null
  current = {
    provider: 'anthropic',
    model: 'claude-haiku-4-5',
    endpoint: '',
    api_key_set: true,
    from_env: [],
    providers: ['mock', 'anthropic', 'openai-compatible'],
  }
  vi.stubGlobal('fetch', fetchMock)
  // The provider is server-wide: never under the profile's path.
  useProfile('personal')
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  useProfile(null)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

async function render() {
  component = mount(ProviderSettings, { target })
  await settle()
}

function button(text: string): HTMLButtonElement | undefined {
  return [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)
}
function field<T extends HTMLElement = HTMLInputElement>(id: string): T {
  return target.querySelector<T>(`#${id}`)!
}
async function type(el: HTMLInputElement, v: string) {
  el.value = v
  el.dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}
async function click(b: HTMLElement | undefined) {
  b!.click()
  await settle()
}
async function save() {
  field<HTMLSelectElement>('provider-provider').form!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await settle()
}

describe('ProviderSettings', () => {
  it('shows the settings, says they apply to every profile, and never the key', async () => {
    await render()
    expect(calls).toEqual([{ method: 'GET', url: '/api/provider', body: undefined }])
    expect(target.textContent).toContain('applies to every profile')
    expect(field<HTMLSelectElement>('provider-provider').value).toBe('anthropic')
    expect(field('provider-model').value).toBe('claude-haiku-4-5')
    expect(field('provider-key-state').textContent?.trim()).toBe('Set.')
    expect(target.querySelector('input[type="password"]')).toBeNull()
  })

  it('replaces the key', async () => {
    await render()
    await click(button('Replace'))
    await type(field('provider-api_key'), 'sk-new')
    await save()
    expect(calls.at(-1)).toEqual({
      method: 'PUT',
      url: '/api/provider',
      body: { provider: 'anthropic', model: 'claude-haiku-4-5', endpoint: '', api_key: 'sk-new' },
    })
    expect(field('provider-key-state').textContent?.trim()).toBe('Set.')
    expect(target.textContent).toContain('Saved.')
  })

  it('clears the key, and leaves it out when untouched', async () => {
    current.provider = 'openai-compatible'
    current.endpoint = 'http://localhost:11434/v1'
    current.model = 'llama3.1:8b'
    await render()
    await type(field('provider-model'), 'qwen3:8b')
    await save()
    expect(calls.at(-1)!.body).toEqual({ provider: 'openai-compatible', model: 'qwen3:8b', endpoint: 'http://localhost:11434/v1' })
    await click(button('Clear'))
    await save()
    expect((calls.at(-1)!.body as { api_key?: string }).api_key).toBe('')
    expect(field('provider-key-state').textContent?.trim()).toBe('Not set.')
  })

  it('makes fields from env vars read-only with a note', async () => {
    current.from_env = ['endpoint', 'api_key']
    await render()
    expect(field('provider-endpoint').disabled).toBe(true)
    expect(field('provider-model').disabled).toBe(false)
    expect(field('provider-endpoint-env').textContent).toContain('WAYPOINT_ENDPOINT')
    expect(field('provider-api_key-env').textContent).toContain('WAYPOINT_API_KEY')
    expect(button('Replace')).toBeUndefined()
    expect(button('Clear')).toBeUndefined()
  })

  it("shows the server's problems beside their fields", async () => {
    await render()
    reply = json(
      {
        error: {
          code: 'invalid',
          message: 'Invalid.',
          problems: [{ field: 'model', problem: "is set by WAYPOINT_MODEL and can't be changed here" }],
        },
      },
      422,
    )
    await type(field('provider-model'), 'other')
    await save()
    expect(field('provider-model-problem').textContent).toBe("Is set by WAYPOINT_MODEL and can't be changed here.")
    expect(field('provider-model').getAttribute('aria-invalid')).toBe('true')
  })

  it('hides the model fields when no model is used', async () => {
    await render()
    const select = field<HTMLSelectElement>('provider-provider')
    select.value = 'mock'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await settle()
    expect(target.querySelector('#provider-model')).toBeNull()
    await save()
    expect(calls.at(-1)!.body).toEqual({ provider: 'mock', model: 'claude-haiku-4-5', endpoint: '' })
  })
})
