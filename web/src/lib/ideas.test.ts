import { describe, expect, it } from 'vitest'
import type { Idea, Project } from './api/types.gen'
import { defaultTarget, projectName, promoteTargets } from './ideas'

function project(id: number, status: Project['status']): Project {
  return {
    id,
    title: `P${id}`,
    why: '',
    definition_of_done: '',
    status,
    created_at: '',
    updated_at: '',
    last_activity_at: '',
  }
}
function idea(projectId: number | null): Idea {
  return {
    id: 1,
    project_id: projectId,
    title: 'I',
    body: '',
    status: 'parked',
    origin_entry_id: null,
    created_at: '',
    updated_at: '',
  }
}

const projects = [
  project(1, 'parked'),
  project(2, 'done'),
  project(3, 'active'),
  project(4, 'paused'),
  project(5, 'active'),
  project(6, 'archived'),
]

describe('promoteTargets', () => {
  it('leaves out done and archived projects and puts active first, then paused, then parked', () => {
    expect(promoteTargets(projects).map((p) => p.id)).toEqual([3, 5, 4, 1])
  })
})

describe('defaultTarget', () => {
  const targets = promoteTargets(projects)
  it("uses the idea's own project", () => {
    expect(defaultTarget(idea(1), targets)).toBe(1)
  })
  it('falls back to the first target when the idea has no project or a done one', () => {
    expect(defaultTarget(idea(null), targets)).toBe(3)
    expect(defaultTarget(idea(2), targets)).toBe(3)
  })
  it('is null when there are no targets', () => {
    expect(defaultTarget(idea(null), [])).toBeNull()
  })
})

describe('projectName', () => {
  it("names the idea's project, or null", () => {
    expect(projectName(idea(4), projects)).toBe('P4')
    expect(projectName(idea(null), projects)).toBeNull()
    expect(projectName(idea(99), projects)).toBeNull()
  })
})
