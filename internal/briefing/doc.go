// Package briefing produces the "Now" briefing (SPEC §7.3): for a project, one
// next action with a reason and a recap of where the user left off. A Briefer
// makes the judgment; the Service loads its input, caches its output in the
// briefings table, and refreshes it when state changes.
package briefing
