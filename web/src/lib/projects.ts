import type { ProjectStatus } from './api/enums.gen'
import type { CreateProjectRequest, Project, UpdateProjectRequest } from './api/types.gen'

export interface ProjectGroup {
  status: ProjectStatus
  label: string
  projects: Project[]
}

const groupOrder: { status: ProjectStatus; label: string }[] = [
  { status: 'active', label: 'Active' },
  { status: 'paused', label: 'Paused' },
  { status: 'parked', label: 'Parked' },
  { status: 'done', label: 'Done' },
  { status: 'archived', label: 'Archived' },
]

// groupByStatus sorts projects into the Projects screen's sections, active
// first. Empty sections are left out; projects keep their order within one.
export function groupByStatus(projects: Project[]): ProjectGroup[] {
  return groupOrder
    .map(({ status, label }) => ({ status, label, projects: projects.filter((p) => p.status === status) }))
    .filter((g) => g.projects.length > 0)
}

// pickable reports whether a project belongs in a project picker (diary
// scope, review, promote targets). Done and archived projects are left out,
// unless it's the one already chosen (keep).
export function pickable(p: Project, keep: number | null = null): boolean {
  return (p.status !== 'done' && p.status !== 'archived') || p.id === keep
}

export function countActive(projects: Project[]): number {
  return projects.filter((p) => p.status === 'active').length
}

// atCap reports whether another project can't become active. An unknown cap
// (settings didn't load) never blocks: the server has the final say.
export function atCap(activeCount: number, cap: number | null): boolean {
  return cap !== null && activeCount >= cap
}

// capMessage explains the active-project cap without making it a failure.
export function capMessage(activeCount: number, cap: number): string {
  const projects = activeCount === 1 ? 'project' : 'projects'
  return `You have ${activeCount} active ${projects}, and the limit is ${cap}. This one can wait in the parking lot until you pause or finish one.`
}

export interface NewProjectForm {
  title: string
  why: string
  definitionOfDone: string
  startActive: boolean
}

// newProjectRequest builds the POST /api/projects body. Blank optional fields
// are left out; the server creates a project parked unless asked otherwise.
export function newProjectRequest(form: NewProjectForm): CreateProjectRequest {
  const req: CreateProjectRequest = { title: form.title.trim() }
  const why = form.why.trim()
  const dod = form.definitionOfDone.trim()
  if (why) req.why = why
  if (dod) req.definition_of_done = dod
  if (form.startActive) req.status = 'active'
  return req
}

// activeProjects are the projects the Now screen briefs, in their list order.
export function activeProjects(projects: Project[]): Project[] {
  return projects.filter((p) => p.status === 'active')
}

export interface ProjectFields {
  title: string
  why: string
  definitionOfDone: string
}

export function projectFields(p: Project): ProjectFields {
  return { title: p.title, why: p.why, definitionOfDone: p.definition_of_done }
}

// projectPatch builds the PATCH /api/projects/{id} body from the edit form:
// only the fields that changed, trimmed. Why and definition of done may be
// cleared to empty. It returns null when nothing changed.
export function projectPatch(p: Project, form: ProjectFields): UpdateProjectRequest | null {
  const req: UpdateProjectRequest = {}
  const title = form.title.trim()
  const why = form.why.trim()
  const dod = form.definitionOfDone.trim()
  if (title !== p.title) req.title = title
  if (why !== p.why) req.why = why
  if (dod !== p.definition_of_done) req.definition_of_done = dod
  return Object.keys(req).length > 0 ? req : null
}

export interface StatusOption {
  status: ProjectStatus
  label: string
  disabled: boolean
}

// statusOptions lists the statuses a project can be set to. Active is
// disabled at the cap unless the project is already active.
export function statusOptions(current: ProjectStatus, activeCount: number, cap: number | null): StatusOption[] {
  const full = current !== 'active' && atCap(activeCount, cap)
  return groupOrder.map(({ status, label }) => ({ status, label, disabled: status === 'active' && full }))
}

// activateCapMessage explains why an existing project can't become active
// right now, without making it a failure.
export function activateCapMessage(activeCount: number, cap: number): string {
  const projects = activeCount === 1 ? 'project' : 'projects'
  return `You have ${activeCount} active ${projects}, and the limit is ${cap}. Pause or finish one when you'd like to make this one active.`
}

// parkedProjects are the projects the Parking lot shows, in their list order.
export function parkedProjects(projects: Project[]): Project[] {
  return projects.filter((p) => p.status === 'parked')
}
