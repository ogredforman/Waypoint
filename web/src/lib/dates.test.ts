import { describe, expect, it } from 'vitest'
import { relativeDay } from './dates'

describe('relativeDay', () => {
  const now = new Date(2026, 9, 3, 9, 30) // 3 Oct 2026, 09:30 local
  const at = (month: number, day: number, hour = 12, year = 2026) => new Date(year, month, day, hour)

  it.each([
    ['earlier today', at(9, 3, 1), 'today'],
    ['late yesterday', at(9, 2, 23), 'yesterday'],
    ['early yesterday', at(9, 2, 0), 'yesterday'],
    ['within the week', at(8, 28), '5 days ago'],
    ['a week ago', at(8, 26), 'Sep 26'],
    ['another year', at(11, 30, 12, 2025), 'Dec 30, 2025'],
  ])('%s', (_, when, want) => {
    expect(relativeDay(when, now, 'en-US')).toBe(want)
  })

  it('takes ISO strings', () => {
    expect(relativeDay(at(9, 3, 8).toISOString(), now, 'en-US')).toBe('today')
  })
})
