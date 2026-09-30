import type { ProjectStatus } from './api/enums.gen'
import type { CreateProjectRequest, Project } from './api/types.gen'

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
]

// groupByStatus sorts projects into the Projects screen's sections, active
// first. Empty sections are left out; projects keep their order within one.
export function groupByStatus(projects: Project[]): ProjectGroup[] {
  return groupOrder
    .map(({ status, label }) => ({ status, label, projects: projects.filter((p) => p.status === status) }))
    .filter((g) => g.projects.length > 0)
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
