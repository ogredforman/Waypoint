import { ApiError } from './api/client'
import type { Provider, UpdateProviderRequest } from './api/types.gen'

// The provider names the server knows (provider.Names).
export const noModel = 'mock'
export const anthropic = 'anthropic'
export const openAICompatible = 'openai-compatible'

// ProviderField is a setting the Model section edits, by its API name.
export type ProviderField = 'provider' | 'model' | 'endpoint' | 'api_key'

// KeyAction is what a save does to the API key: keep it, replace it with
// value, or clear it. The key itself is never shown.
export type KeyAction = { kind: 'keep' } | { kind: 'replace'; value: string } | { kind: 'clear' }

// ProviderForm is what the Model section edits.
export interface ProviderForm {
  provider: string
  model: string
  endpoint: string
  key: KeyAction
}

export function providerForm(p: Provider): ProviderForm {
  return { provider: p.provider || noModel, model: p.model, endpoint: p.endpoint, key: { kind: 'keep' } }
}

// providerLabel is how the select names a provider.
export function providerLabel(name: string): string {
  switch (name) {
    case noModel:
      return 'None (no model; only simple line prefixes are read)'
    case anthropic:
      return 'Anthropic'
    case openAICompatible:
      return 'OpenAI-compatible (Ollama, LM Studio, llama.cpp, OpenAI)'
    default:
      return name
  }
}

// fieldPlaceholder is the hint shown in an empty model or endpoint field:
// the default the provider uses, or what to type.
export function fieldPlaceholder(provider: string, field: 'model' | 'endpoint'): string {
  if (provider === anthropic) return field === 'model' ? 'claude-haiku-4-5' : 'https://api.anthropic.com/v1'
  if (provider === openAICompatible) return field === 'model' ? 'e.g. llama3.1:8b' : 'e.g. http://localhost:11434/v1'
  return ''
}

// fromEnv says whether a WAYPOINT_* env var sets the field, so it can't change here.
export function fromEnv(p: Provider, field: ProviderField): boolean {
  return p.from_env.includes(field)
}

export function envNote(field: ProviderField): string {
  return `Set by WAYPOINT_${field.toUpperCase()}, so it can't be changed here.`
}

// ProviderProblems maps a field to what's wrong with it; "form" holds the rest.
export type ProviderProblems = Partial<Record<ProviderField | 'form', string>>

// providerRequest is the PUT body, or null when nothing changed. The key is
// sent only to replace or clear it. Settings no model could run with are
// problems, found here so each sits beside its field.
export function providerRequest(
  saved: Provider,
  form: ProviderForm,
): { request: UpdateProviderRequest | null; problems: ProviderProblems } {
  const provider = form.provider.trim() || noModel
  const model = form.model.trim()
  const endpoint = form.endpoint.trim()
  const key = form.key.kind === 'replace' ? form.key.value.trim() : null

  const problems: ProviderProblems = {}
  if (form.key.kind === 'replace' && key === '') problems.api_key = 'Enter the new key, or cancel.'
  if (provider === openAICompatible) {
    if (endpoint === '') problems.endpoint = 'Enter the address of the server, ending in /v1.'
    if (model === '') problems.model = 'Enter the name of the model to use.'
  }
  if (provider === anthropic && !problems.api_key) {
    const keyAfter = form.key.kind === 'replace' || (form.key.kind === 'keep' && saved.api_key_set)
    if (!keyAfter) problems.api_key = 'Anthropic needs an API key.'
  }
  if (Object.keys(problems).length > 0) return { request: null, problems }

  const request: UpdateProviderRequest = { provider, model, endpoint }
  if (form.key.kind === 'replace') request.api_key = key!
  if (form.key.kind === 'clear') request.api_key = ''
  const same =
    provider === (saved.provider || noModel) &&
    model === saved.model &&
    endpoint === saved.endpoint &&
    request.api_key === undefined
  return { request: same ? null : request, problems }
}

const fields: ProviderField[] = ['provider', 'model', 'endpoint', 'api_key']

// providerProblems turns a failed save into messages per field; other
// errors go under "form".
export function providerProblems(err: unknown): ProviderProblems {
  if (!(err instanceof ApiError)) return { form: "Couldn't save the model settings." }
  if (err.code !== 'invalid' || err.problems.length === 0) return { form: err.message }
  const out: ProviderProblems = {}
  for (const p of err.problems) {
    const text = p.problem.charAt(0).toUpperCase() + p.problem.slice(1) + '.'
    const field = fields.find((f) => f === p.field) ?? 'form'
    out[field] = out[field] ? `${out[field]} ${text}` : text
  }
  return out
}
