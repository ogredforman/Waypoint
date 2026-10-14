import { describe, expect, it } from 'vitest'
import { ApiError } from './api/client'
import type { Provider } from './api/types.gen'
import { providerForm, providerProblems, providerRequest } from './provider'

function provider(over: Partial<Provider> = {}): Provider {
  return {
    provider: 'anthropic',
    model: '',
    endpoint: '',
    api_key_set: true,
    from_env: [],
    providers: ['mock', 'anthropic', 'openai-compatible'],
    ...over,
  }
}

describe('providerRequest', () => {
  it('is null when nothing changed', () => {
    const saved = provider()
    expect(providerRequest(saved, providerForm(saved))).toEqual({ request: null, problems: {} })
  })

  it('keeps the key unless it is replaced or cleared', () => {
    const saved = provider()
    const form = { ...providerForm(saved), model: ' claude-sonnet-4-5 ' }
    expect(providerRequest(saved, form).request).toEqual({ provider: 'anthropic', model: 'claude-sonnet-4-5', endpoint: '' })
    form.key = { kind: 'replace', value: ' sk-new ' }
    expect(providerRequest(saved, form).request?.api_key).toBe('sk-new')
    form.key = { kind: 'clear' }
    // Anthropic can't run without a key, so clearing it there is a problem.
    expect(providerRequest(saved, form).problems).toEqual({ api_key: 'Anthropic needs an API key.' })
    expect(providerRequest(saved, { ...form, provider: 'mock' }).request?.api_key).toBe('')
  })

  it('counts clearing the key alone as a change', () => {
    const saved = provider({ provider: 'mock' })
    const form = { ...providerForm(saved), key: { kind: 'clear' as const } }
    expect(providerRequest(saved, form).request).toEqual({ provider: 'mock', model: '', endpoint: '', api_key: '' })
  })

  it('finds settings no model could run with', () => {
    const saved = provider({ provider: 'mock', api_key_set: false })
    expect(providerRequest(saved, { ...providerForm(saved), provider: 'openai-compatible' }).problems).toEqual({
      endpoint: 'Enter the address of the server, ending in /v1.',
      model: 'Enter the name of the model to use.',
    })
    expect(providerRequest(saved, { ...providerForm(saved), provider: 'anthropic' }).problems).toEqual({
      api_key: 'Anthropic needs an API key.',
    })
    const blank = { ...providerForm(saved), key: { kind: 'replace' as const, value: ' ' } }
    expect(providerRequest(saved, blank)).toEqual({ request: null, problems: { api_key: 'Enter the new key, or cancel.' } })
  })
})

describe('providerProblems', () => {
  it('puts each problem under its field', () => {
    const err = new ApiError(422, 'invalid', 'Invalid.', [
      { field: 'model', problem: "is set by WAYPOINT_MODEL and can't be changed here" },
      { field: 'other', problem: 'is odd' },
    ])
    expect(providerProblems(err)).toEqual({
      model: "Is set by WAYPOINT_MODEL and can't be changed here.",
      form: 'Is odd.',
    })
  })

  it('puts other errors under the form', () => {
    expect(providerProblems(new ApiError(500, 'internal', 'Something went wrong.'))).toEqual({ form: 'Something went wrong.' })
    expect(providerProblems(new Error('x'))).toEqual({ form: "Couldn't save the model settings." })
  })
})
