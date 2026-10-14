<script lang="ts">
  import { onMount } from 'svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'
  import { fieldClass } from './ui/field'
  import { ApiError, getProvider, updateProvider } from '../lib/api/client'
  import type { Provider } from '../lib/api/types.gen'
  import {
    envNote,
    fieldPlaceholder,
    fromEnv,
    noModel,
    providerForm,
    providerLabel,
    providerProblems,
    providerRequest,
    type ProviderForm,
    type ProviderProblems,
  } from '../lib/provider'

  let saved = $state<Provider | null>(null)
  let form = $state<ProviderForm>({ provider: noModel, model: '', endpoint: '', key: { kind: 'keep' } })
  let loadError = $state('')
  let saving = $state(false)
  let problems = $state<ProviderProblems>({})
  let notice = $state('')

  const changed = $derived(saved !== null && providerRequest(saved, form).request !== null)
  const pendingProblems = $derived(saved !== null && Object.keys(providerRequest(saved, form).problems).length > 0)
  const usesModel = $derived(form.provider !== noModel)

  async function load() {
    try {
      saved = await getProvider()
      form = providerForm(saved)
      loadError = ''
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load the model settings."
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!saved || saving) return
    const { request, problems: local } = providerRequest(saved, form)
    problems = local
    if (!request) return
    saving = true
    notice = ''
    try {
      saved = await updateProvider(request)
      form = providerForm(saved)
      notice = 'Saved. Every profile uses it from the next entry on.'
    } catch (err) {
      problems = providerProblems(err)
    } finally {
      saving = false
    }
  }

  function edited() {
    notice = ''
    problems = {}
  }

  function setKey(key: ProviderForm['key']) {
    form.key = key
    edited()
  }

  function describedBy(field: 'provider' | 'model' | 'endpoint' | 'api_key', ...extra: string[]): string | undefined {
    const ids = [...extra]
    if (saved && fromEnv(saved, field)) ids.push(`provider-${field}-env`)
    if (problems[field]) ids.push(`provider-${field}-problem`)
    return ids.length > 0 ? ids.join(' ') : undefined
  }

  onMount(load)
</script>

{#snippet envAndProblem(field: 'provider' | 'model' | 'endpoint' | 'api_key')}
  {#if saved && fromEnv(saved, field)}
    <p id="provider-{field}-env" class="mt-1 text-sm text-ink-muted">{envNote(field)}</p>
  {/if}
  {#if problems[field]}
    <p id="provider-{field}-problem" class="mt-1 text-sm text-ink">{problems[field]}</p>
  {/if}
{/snippet}

<section aria-labelledby="model-heading" class="space-y-3">
  <div>
    <h2 id="model-heading" class="text-lg font-semibold">Model</h2>
    <p class="text-sm text-ink-muted">The model that reads your entries. It applies to every profile.</p>
  </div>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if saved === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    <Panel>
      <form class="space-y-6 p-5" onsubmit={save} oninput={edited} novalidate>
        <div>
          <label for="provider-provider" class="text-sm font-medium text-ink">Provider</label>
          <select
            id="provider-provider"
            class={[fieldClass, 'max-w-md']}
            bind:value={form.provider}
            onchange={edited}
            disabled={fromEnv(saved, 'provider')}
            aria-describedby={describedBy('provider')}
            aria-invalid={problems.provider ? 'true' : undefined}
          >
            {#each saved.providers as name (name)}
              <option value={name}>{providerLabel(name)}</option>
            {/each}
          </select>
          {@render envAndProblem('provider')}
        </div>

        {#if usesModel}
          <div>
            <label for="provider-model" class="text-sm font-medium text-ink">Model</label>
            <Input
              id="provider-model"
              class="max-w-md"
              autocomplete="off"
              spellcheck="false"
              bind:value={form.model}
              placeholder={fieldPlaceholder(form.provider, 'model')}
              disabled={fromEnv(saved, 'model')}
              aria-describedby={describedBy('model')}
              aria-invalid={problems.model ? 'true' : undefined}
            />
            {@render envAndProblem('model')}
          </div>

          <div>
            <label for="provider-endpoint" class="text-sm font-medium text-ink">Endpoint</label>
            <Input
              id="provider-endpoint"
              type="url"
              class="max-w-md"
              autocomplete="off"
              spellcheck="false"
              bind:value={form.endpoint}
              placeholder={fieldPlaceholder(form.provider, 'endpoint')}
              disabled={fromEnv(saved, 'endpoint')}
              aria-describedby={describedBy('endpoint')}
              aria-invalid={problems.endpoint ? 'true' : undefined}
            />
            {@render envAndProblem('endpoint')}
          </div>

          <div>
            <p class="text-sm font-medium text-ink" id="provider-key-label">API key</p>
            {#if form.key.kind === 'replace'}
              <label for="provider-api_key" class="sr-only">New API key</label>
              <div class="flex items-center gap-2">
                <Input
                  id="provider-api_key"
                  type="password"
                  class="max-w-md"
                  autocomplete="off"
                  bind:value={form.key.value}
                  aria-describedby={describedBy('api_key')}
                  aria-invalid={problems.api_key ? 'true' : undefined}
                />
                <Button variant="quiet" size="sm" onclick={() => setKey({ kind: 'keep' })}>Cancel</Button>
              </div>
            {:else}
              <div class="flex items-center gap-2">
                <p class="text-sm text-ink-muted" id="provider-key-state" aria-labelledby="provider-key-label" role="status">
                  {#if form.key.kind === 'clear'}
                    Removed when you save.
                  {:else if saved.api_key_set}
                    Set.
                  {:else}
                    Not set.
                  {/if}
                </p>
                {#if !fromEnv(saved, 'api_key')}
                  {#if form.key.kind === 'clear'}
                    <Button variant="quiet" size="xs" onclick={() => setKey({ kind: 'keep' })}>Keep it</Button>
                  {:else}
                    <Button variant="quiet" size="xs" onclick={() => setKey({ kind: 'replace', value: '' })}>
                      {saved.api_key_set ? 'Replace' : 'Add'}
                    </Button>
                    {#if saved.api_key_set}
                      <Button variant="quiet" size="xs" onclick={() => setKey({ kind: 'clear' })}>Clear</Button>
                    {/if}
                  {/if}
                {/if}
              </div>
            {/if}
            {@render envAndProblem('api_key')}
          </div>
        {/if}

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={(!changed && !pendingProblems) || saving}>{saving ? 'Saving…' : 'Save'}</Button>
          <p class="text-sm text-ink-muted" aria-live="polite">{problems.form ?? notice}</p>
        </div>
      </form>
    </Panel>
  {/if}
</section>
