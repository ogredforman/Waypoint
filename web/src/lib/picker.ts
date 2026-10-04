import type { Project } from './api/types.gen'
import { filterProjects } from './projects'

// PickerOption is one choice in a project picker: a project, or null for no project.
export interface PickerOption {
  id: number | null
  label: string
}

export const noProjectLabel = 'No project'

// pickerOptions are the choices a project picker shows for a typed query.
// "No project" always stays, so a choice can be cleared whatever is typed. A
// chosen project that isn't in the list (not loaded yet) shows as
// missingLabel until it is. projects come in already filtered and ordered by
// the caller (see pickable).
export function pickerOptions(
  projects: Project[],
  query: string,
  value: number | null,
  missingLabel: string,
): PickerOption[] {
  const opts: PickerOption[] = [{ id: null, label: noProjectLabel }]
  if (value !== null && !projects.some((p) => p.id === value) && query.trim() === '') {
    opts.push({ id: value, label: missingLabel })
  }
  for (const p of filterProjects(projects, query)) opts.push({ id: p.id, label: p.title })
  return opts
}

// selectedLabel is the text the closed picker shows for value.
export function selectedLabel(projects: Project[], value: number | null, missingLabel: string): string {
  if (value === null) return noProjectLabel
  return projects.find((p) => p.id === value)?.title ?? missingLabel
}

// step moves the highlighted option by delta, wrapping at either end. With
// nothing highlighted (-1), down goes to the first and up to the last.
export function step(index: number, delta: number, count: number): number {
  if (count === 0) return -1
  if (index < 0) return delta > 0 ? 0 : count - 1
  return (((index + delta) % count) + count) % count
}
