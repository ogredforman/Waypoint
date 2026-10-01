<script lang="ts">
  import DiaryInput from './DiaryInput.svelte'
  import EntryTimeline from './EntryTimeline.svelte'
</script>

<!--
  The diary itself: shared by the side panel, the narrow-window overlay, and the full-page /diary
  view, so it must work at any of those widths (ADR 0004). App mounts at most one copy at a time;
  state that must outlive a panel or overlay closing lives in shared modules (diaryDraft for the
  entry being written, timeline for the loaded entries), not here.
-->
<div class="space-y-6">
  <DiaryInput />
  <EntryTimeline />
</div>
