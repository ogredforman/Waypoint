import { expect, test } from '@playwright/test'

// The core loop end to end with the mock extractor (SPEC §10.4, §12): create a project, write an
// entry with todo:/blocked: lines, apply the proposed changes, and see the task, the blocker,
// and the briefing update without a reload.
test('write → review → apply updates the project and the briefing', async ({ page }) => {
  const project = 'Garden shed'
  const task = 'Measure the back wall'
  const blocker = 'Waiting on the timber delivery'

  // With no projects yet, the Projects screen opens straight onto the new-project form.
  await page.goto('/projects')
  await page.getByLabel('Title').fill(project)
  await page.getByLabel('Make it active and start working on it').check()
  await page.getByRole('button', { name: 'Create project' }).click()
  await expect(page.getByRole('main').getByRole('link', { name: project })).toBeVisible()

  // A new project has nothing to start yet.
  await page.goto('/')
  const briefing = page.getByRole('article', { name: project })
  await expect(briefing.getByText('Nothing here is ready to start yet.')).toBeVisible()

  // Write the entry in the diary panel, about the new project.
  await page.getByLabel('About').selectOption({ label: project })
  await page.getByLabel('Diary entry').fill(`Cleared the corner today.\ntodo: ${task}\nblocked: ${blocker}`)
  await page.getByLabel('Diary entry').press('Control+Enter')

  // The run finishes and its changeset shows up for review; accept everything.
  const review = page.getByRole('region', { name: 'Proposed changes' })
  await expect(review).toBeVisible()
  await review.getByRole('button', { name: 'Accept all' }).click()
  await expect(review).toBeHidden()

  // The briefing updates live: the new task is the next action, the blocker is in the way.
  await expect(briefing.getByRole('link', { name: task })).toBeVisible()
  await expect(briefing.getByRole('listitem').filter({ hasText: blocker })).toBeVisible()

  // The project screen lists both.
  await briefing.getByRole('link', { name: project }).click()
  await expect(page.getByRole('region', { name: 'Tasks' }).getByRole('link', { name: task })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Blockers' }).getByText(blocker, { exact: true })).toBeVisible()
})
