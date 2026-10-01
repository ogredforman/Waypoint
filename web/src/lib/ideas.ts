import type { Idea, Project } from './api/types.gen'

const targetOrder = { active: 0, paused: 1, parked: 2, done: 3 } as const

// promoteTargets lists the projects an idea can become a task in: any that
// isn't done, active first, then paused, then parked, keeping list order
// within a status.
export function promoteTargets(projects: Project[]): Project[] {
  return projects
    .filter((p) => p.status !== 'done')
    .map((p, i) => ({ p, i }))
    .sort((a, b) => targetOrder[a.p.status] - targetOrder[b.p.status] || a.i - b.i)
    .map(({ p }) => p)
}

// defaultTarget picks the project a promoted task goes into: the idea's own
// project when it's a target, otherwise the first target, or null if none.
export function defaultTarget(idea: Idea, targets: Project[]): number | null {
  if (idea.project_id !== null && targets.some((p) => p.id === idea.project_id)) return idea.project_id
  return targets[0]?.id ?? null
}

// projectName names the idea's project for the Parking lot, or null when the
// idea has none (or the project wasn't loaded).
export function projectName(idea: Idea, projects: Project[]): string | null {
  if (idea.project_id === null) return null
  return projects.find((p) => p.id === idea.project_id)?.title ?? null
}
