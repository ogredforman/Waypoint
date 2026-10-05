import type { Idea, Project, UpdateIdeaRequest } from './api/types.gen'
import { pickable } from './projects'

const targetOrder = { active: 0, paused: 1, parked: 2, done: 3, archived: 4 } as const

// promoteTargets lists the projects an idea can become a task in: any that
// isn't done or archived, active first, then paused, then parked, keeping list order
// within a status.
export function promoteTargets(projects: Project[]): Project[] {
  return projects
    .filter((p) => pickable(p))
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

// IdeaForm is the edit form's fields; the body is markdown from the rich text editor.
export type IdeaForm = { title: string; body: string }

// ideaPatch builds the PATCH /api/ideas/{id} body from the edit form: only
// the fields that changed, trimmed. It returns null when nothing changed.
export function ideaPatch(idea: Idea, form: IdeaForm): UpdateIdeaRequest | null {
  const req: UpdateIdeaRequest = {}
  const title = form.title.trim()
  const body = form.body.trim()
  if (title !== idea.title) req.title = title
  if (body !== idea.body) req.body = body
  return Object.keys(req).length > 0 ? req : null
}
