// FakeEventSource stands in for the browser's EventSource in tests: emit
// sends a server event, and open/fail drive the connection state.
export class FakeEventSource extends EventTarget {
  static all: FakeEventSource[] = []
  readyState = 0
  closed = false

  constructor(readonly url: string) {
    super()
    FakeEventSource.all.push(this)
  }

  static latest(): FakeEventSource {
    const s = FakeEventSource.all.at(-1)
    if (!s) throw new Error('no event source opened')
    return s
  }

  open() {
    this.readyState = 1
    this.dispatchEvent(new Event('open'))
  }

  // fail drops the connection; final means the browser gave up (readyState CLOSED).
  fail(final = false) {
    this.readyState = final ? 2 : 0
    this.dispatchEvent(new Event('error'))
  }

  emit(name: string, data: unknown) {
    this.dispatchEvent(new MessageEvent(name, { data: typeof data === 'string' ? data : JSON.stringify(data) }))
  }

  close() {
    this.closed = true
    this.readyState = 2
  }
}

export function fakeOpen(url: string): EventSource {
  return new FakeEventSource(url) as unknown as EventSource
}
