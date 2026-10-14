<script lang="ts">
  import { onMount } from 'svelte'
  import { getProvider } from '../lib/api/client'
  import type { Project } from '../lib/api/types.gen'
  import { paths } from '../lib/router'
  import NewProjectForm from './NewProjectForm.svelte'

  // The Now screen of a profile with no projects yet (SPEC §11): a short welcome
  // and the form for the first project, which starts active so its briefing
  // shows straight away.

  let { oncreated }: { oncreated: (p: Project) => void } = $props()

  // noModel is set when no model is set up, so only prefixed lines are read.
  let noModel = $state(false)

  onMount(async () => {
    try {
      noModel = (await getProvider()).provider === 'mock'
    } catch {
      // Without an answer the hint just stays out; the welcome works either way.
    }
  })
</script>

<div class="space-y-4">
  <div class="space-y-2 text-ink-muted">
    <p>
      Welcome. Start with one project, something you'd like to move forward. It can be small, and the details can wait.
    </p>
    <p>
      Then write about it in the diary: what you did, what's in the way, what's next. Waypoint turns that into tasks for
      you to review, and this page shows the next step.
    </p>
    {#if noModel}
      <p>
        No model is set up yet, so for now Waypoint reads lines that start with <code>todo:</code>, <code>done:</code>,
        <code>blocked:</code>, or <code>idea:</code>. You can choose one in
        <a href={paths.settings} class="text-accent underline underline-offset-2">Settings</a> whenever you like.
      </p>
    {/if}
  </div>

  <NewProjectForm activeCount={0} cap={null} startActive {oncreated} />
</div>
