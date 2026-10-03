// Dates as people say them, for lists that read at a glance.

const dayMs = 24 * 60 * 60 * 1000

// relativeDay says when a moment was, by calendar day in the local time zone:
// "today", "yesterday", "3 days ago" within the week, then the date ("Sep 28",
// with the year if it isn't this year's).
export function relativeDay(at: string | Date, now: Date = new Date(), locale?: string): string {
  const then = new Date(at)
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const days = Math.round((startOfDay(now) - startOfDay(then)) / dayMs)
  if (days >= 0 && days < 7) {
    return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(-days, 'day')
  }
  const sameYear = then.getFullYear() === now.getFullYear()
  return then.toLocaleDateString(locale, sameYear ? { month: 'short', day: 'numeric' } : { dateStyle: 'medium' })
}
