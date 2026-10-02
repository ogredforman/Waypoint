<script lang="ts">
  import { onMount } from 'svelte'
  import Button from '../components/ui/Button.svelte'
  import Input from '../components/ui/Input.svelte'
  import Panel from '../components/ui/Panel.svelte'
  import { ApiError, getSettings, listProjects, updateSettings } from '../lib/api/client'
  import type { Settings } from '../lib/api/types.gen'
  import { latest, liveReload } from '../lib/live'
  import { pageHeading } from '../lib/navigation.svelte'
  import { countActive } from '../lib/projects'
  import {
    capNote,
    fieldProblems,
    parseCap,
    questionChoices,
    settingsForm,
    settingsPatch,
    type FieldProblems,
    type SettingsForm,
  } from '../lib/settings'

  let saved = $state<Settings | null>(null)
  let form = $state<SettingsForm>({ cap: '', questions: 0 })
  let loadError = $state('')
  let activeCount = $state<number | null>(null)
  let saving = $state(false)
  let problems = $state<FieldProblems>({})
  let notice = $state('')

  // Save stays enabled for an invalid cap, so pressing it explains the problem.
  const changed = $derived(saved !== null && settingsPatch(saved, form).patch !== null)
  const invalidCap = $derived(saved !== null && parseCap(form.cap) === null)
  const note = $derived(capNote(activeCount, parseCap(form.cap)))

  const counts = latest()

  async function load() {
    try {
      saved = await getSettings()
      form = settingsForm(saved)
      loadError = ''
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load your settings."
    }
  }

  // The active-project count only feeds the note about a lowered cap.
  async function loadCount() {
    const current = counts()
    try {
      const n = countActive(await listProjects())
      if (current()) activeCount = n
    } catch {
      if (current()) activeCount = null
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!saved || saving) return
    const { patch, problems: local } = settingsPatch(saved, form)
    problems = local
    if (!patch) return
    saving = true
    notice = ''
    try {
      saved = await updateSettings(patch)
      form = settingsForm(saved)
      notice = 'Saved.'
    } catch (err) {
      problems = fieldProblems(err)
    } finally {
      saving = false
    }
  }

  function edited() {
    notice = ''
    problems = {}
  }

  onMount(() => {
    load()
    loadCount()
    return liveReload(loadCount)
  })
</script>

<!--
  Each group of settings is its own <section> under the heading; later ones
  (profiles, appearance, the extraction provider) go after "Focus".
-->
<section class="space-y-8">
  <div>
    <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Settings</h1>
    <p class="mt-2 text-ink-muted">Preferences for how Waypoint keeps you focused.</p>
  </div>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if saved === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    <section aria-labelledby="focus-heading" class="space-y-3">
      <h2 id="focus-heading" class="text-lg font-semibold">Focus</h2>
      <Panel>
        <form class="space-y-6 p-5" onsubmit={save} oninput={edited} novalidate>
          <div>
            <label for="setting-cap" class="text-sm font-medium text-ink">Active projects at once</label>
            <p id="setting-cap-hint" class="text-sm text-ink-muted">
              How many projects can be active together. Others wait, parked, until one is paused or done.
            </p>
            <Input
              id="setting-cap"
              type="number"
              min="1"
              step="1"
              inputmode="numeric"
              class="max-w-32"
              bind:value={form.cap}
              aria-describedby={problems.active_project_cap ? 'setting-cap-hint setting-cap-problem' : 'setting-cap-hint'}
              aria-invalid={problems.active_project_cap ? 'true' : undefined}
            />
            {#if problems.active_project_cap}
              <p id="setting-cap-problem" class="mt-1 text-sm text-ink">{problems.active_project_cap}</p>
            {/if}
            {#if note}
              <p class="mt-2 text-sm text-ink-muted" role="status">{note}</p>
            {/if}
          </div>

          <fieldset aria-describedby={problems.questions_per_entry ? 'setting-questions-problem' : undefined}>
            <legend class="text-sm font-medium text-ink">Follow-up questions per entry</legend>
            <p class="text-sm text-ink-muted">After reading an entry, Waypoint may ask about what wasn't clear.</p>
            <div class="mt-2 space-y-1">
              {#each questionChoices as choice (choice.value)}
                <label class="flex items-baseline gap-2 text-sm">
                  <input type="radio" name="questions" value={choice.value} bind:group={form.questions} class="accent-accent" />
                  <span class="text-ink">{choice.label}</span>
                  <span class="text-ink-muted">{choice.hint}</span>
                </label>
              {/each}
            </div>
            {#if problems.questions_per_entry}
              <p id="setting-questions-problem" class="mt-1 text-sm text-ink">{problems.questions_per_entry}</p>
            {/if}
          </fieldset>

          <div class="flex items-center gap-3">
            <Button type="submit" disabled={(!changed && !invalidCap) || saving}>{saving ? 'Saving…' : 'Save'}</Button>
            <p class="text-sm text-ink-muted" aria-live="polite">{problems.form ?? notice}</p>
          </div>
        </form>
      </Panel>
    </section>
  {/if}
</section>
