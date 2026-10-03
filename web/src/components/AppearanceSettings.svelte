<script lang="ts">
  import { onDestroy } from 'svelte'
  import { updateSettings } from '../lib/api/client'
  import type { Settings } from '../lib/api/types.gen'
  import {
    appearanceForm,
    appearancePatch,
    diaryFontChoices,
    fonts,
    interfaceFontChoices,
    setAppearance,
    themeChoices,
    type Appearance,
  } from '../lib/appearance'
  import { profiles } from '../lib/profile.svelte'
  import Panel from './ui/Panel.svelte'

  // The Appearance section of Settings: the profile's theme, shown as swatches
  // in each theme's own colours, and its fonts, each with a preview. A choice
  // applies to the whole app at once and is saved straight away; a failed save
  // puts the last saved choice back.
  let { settings, onsaved }: { settings: Settings; onsaved: (s: Settings) => void } = $props()

  // Starts from the saved settings; after that, only choices and saves here change it.
  // svelte-ignore state_referenced_locally
  let form = $state<Appearance>(appearanceForm(settings))
  // The appearance the server last confirmed.
  // svelte-ignore state_referenced_locally
  let committed = appearanceForm(settings)
  let saving = false
  let notice = $state('')
  // After Settings closes (or the profile switches), a save that answers late
  // must not touch the page: another profile's appearance may be on it by then.
  let destroyed = false
  onDestroy(() => (destroyed = true))

  function choose(change: Partial<Appearance>) {
    form = { ...form, ...change }
    notice = ''
    setAppearance(form)
    void persist()
  }

  // persist runs one save at a time, so quick choices can't land out of order;
  // whatever was chosen while a save was in flight goes in the next one.
  async function persist() {
    if (saving) return
    saving = true
    try {
      let patch: ReturnType<typeof appearancePatch>
      while (!destroyed && (patch = appearancePatch(committed, form))) {
        try {
          const saved = await updateSettings(patch)
          if (destroyed) return
          committed = appearanceForm(saved)
          onsaved(saved)
          notice = 'Saved.'
        } catch {
          if (destroyed) return
          form = { ...committed }
          setAppearance(committed)
          notice = "Couldn't save that, so the previous choice is back."
          return
        }
      }
    } finally {
      saving = false
    }
  }
</script>

<section aria-labelledby="appearance-heading" class="space-y-3">
  <div>
    <h2 id="appearance-heading" class="text-lg font-semibold">Appearance</h2>
    {#if profiles.active}
      <p class="text-sm text-ink-muted">For the {profiles.active.name} profile.</p>
    {/if}
  </div>
  <Panel>
    <div class="space-y-6 p-5">
      <fieldset>
        <legend class="text-sm font-medium text-ink">Theme</legend>
        <p class="text-sm text-ink-muted">Colours for the whole app.</p>
        <div class="mt-2 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {#each themeChoices as choice (choice.value)}
            <label
              class="flex cursor-pointer flex-col gap-2 rounded-card border border-line p-3 has-checked:border-accent has-checked:shadow-focus"
            >
              <span class="flex items-center gap-2 text-sm text-ink">
                <input
                  type="radio"
                  name="theme"
                  value={choice.value}
                  checked={form.theme === choice.value}
                  onchange={() => choose({ theme: choice.value })}
                  class="accent-accent"
                />
                <span class="font-medium">{choice.label}</span>
                <span class="text-ink-muted">{choice.mode}</span>
              </span>
              <span class="text-xs text-ink-muted">{choice.description}</span>
              <span
                data-theme={choice.value}
                data-testid="theme-swatch"
                class="block rounded-control border border-line bg-canvas p-2"
                aria-hidden="true"
              >
                <span class="block rounded-control bg-surface p-2">
                  <span class="block text-sm font-semibold text-ink">Next action</span>
                  <span class="block text-xs text-ink-muted">Where you left off</span>
                  <span class="mt-2 flex items-center gap-2">
                    <span class="rounded-control bg-linear-to-b from-accent-strong to-accent-deep px-2 py-0.5 text-xs text-on-accent">
                      Start
                    </span>
                    <span class="text-xs text-accent">Details</span>
                  </span>
                </span>
              </span>
            </label>
          {/each}
        </div>
      </fieldset>

      <fieldset>
        <legend class="text-sm font-medium text-ink">Interface font</legend>
        <p class="text-sm text-ink-muted">Used for menus, buttons, and everything outside the diary.</p>
        <div class="mt-2 grid gap-1 sm:grid-cols-2">
          {#each interfaceFontChoices as choice (choice.value)}
            <label class="flex items-center gap-2 text-sm text-ink">
              <input
                type="radio"
                name="interface-font"
                value={choice.value}
                checked={form.interface_font === choice.value}
                onchange={() => choose({ interface_font: choice.value })}
                class="accent-accent"
              />
              <span style:font-family={choice.stack}>{choice.label}</span>
            </label>
          {/each}
        </div>
        <div
          class="mt-3 rounded-card border border-line bg-canvas p-4 text-ink"
          data-theme={form.theme}
          style:font-family={fonts[form.interface_font].stack}
          data-testid="interface-preview"
          aria-label="Interface font preview"
          role="figure"
        >
          <p class="text-xs tracking-wide text-ink-muted uppercase">Next action</p>
          <p class="mt-1 font-semibold text-ink">Write the README's install section</p>
          <p class="mt-1 text-sm text-ink-muted">About 20 minutes. You left off after the build steps.</p>
          <span class="mt-3 inline-block rounded-control bg-accent-strong px-3 py-1 text-sm text-on-accent">Start</span>
        </div>
      </fieldset>

      <fieldset>
        <legend class="text-sm font-medium text-ink">Diary font</legend>
        <p class="text-sm text-ink-muted">Used for writing and reading diary entries.</p>
        <div class="mt-2 grid gap-1 sm:grid-cols-2">
          {#each diaryFontChoices as choice (choice.value)}
            <label class="flex items-center gap-2 text-sm text-ink">
              <input
                type="radio"
                name="diary-font"
                value={choice.value}
                checked={form.diary_font === choice.value}
                onchange={() => choose({ diary_font: choice.value })}
                class="accent-accent"
              />
              <span style:font-family={choice.stack}>{choice.label}</span>
              {#if choice.value === 'literata'}<span class="text-ink-muted">(serif)</span>{/if}
            </label>
          {/each}
        </div>
        <p
          class="mt-3 rounded-card border border-line bg-canvas p-4 leading-relaxed text-ink"
          data-theme={form.theme}
          style:font-family={fonts[form.diary_font].stack}
          data-testid="diary-preview"
          aria-label="Diary font preview"
          role="figure"
        >
          Got the build working at last. Tomorrow I want to write the install steps, but I'm not sure yet
          whether to cover Windows too.
        </p>
      </fieldset>

      <p class="min-h-5 text-sm text-ink-muted" aria-live="polite">{notice}</p>
    </div>
  </Panel>
</section>
