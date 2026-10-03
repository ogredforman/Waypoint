import { mount } from 'svelte'
import './fonts'
import './app.css'
import App from './App.svelte'
import { chooseProfile, waitingMessage } from './lib/profile.svelte'

const target = document.getElementById('app')
if (!target) throw new Error('missing #app element')

// Every API path names a profile, so pick one before anything asks. Until the
// server answers, say so calmly instead of showing a blank page.
void chooseProfile(undefined, undefined, () => {
  if (target.childElementCount === 0) {
    const p = document.createElement('p')
    p.className = 'p-gutter text-ink-muted'
    p.setAttribute('role', 'status')
    p.textContent = waitingMessage
    target.replaceChildren(p)
  }
}).then(() => {
  target.replaceChildren()
  mount(App, { target })
})
