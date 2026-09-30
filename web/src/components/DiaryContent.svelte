<!--
  The diary itself: shared by the side panel, the narrow-window overlay, and the full-page /diary
  view, so it must work at any of those widths (ADR 0004). The input arrives in 1.17 and the
  timeline in 1.18. App mounts at most one copy at a time; keep any state that must outlive a
  panel or overlay closing (a draft entry, the loaded timeline) in a shared module, not here.
-->
<div class="space-y-3 text-sm text-ink-muted">
  <p>Write about what you did, what's in the way, or what you're thinking. Waypoint will turn it into tasks you can review.</p>
  <p>Your entries will line up here, newest first.</p>
</div>
