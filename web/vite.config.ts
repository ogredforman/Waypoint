/// <reference types="vitest/config" />
import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// The build goes straight into the Go embed directory.
const outDir = fileURLToPath(new URL('../internal/web/dist', import.meta.url))

// The Go server's port; matches config.DefaultPort unless WAYPOINT_PORT overrides it.
// Vite can't see -port or config.toml, so a custom port for `make dev` must come from WAYPOINT_PORT.
const apiPort = process.env.WAYPOINT_PORT || '7420'

// emptyOutDir deletes dist/.gitkeep, which is committed so `go build` works
// before the first frontend build. Put it back so a build leaves git clean.
function keepGitkeep(): Plugin {
  return {
    name: 'waypoint-keep-gitkeep',
    apply: 'build',
    closeBundle() {
      writeFileSync(`${outDir}/.gitkeep`, '')
    },
  }
}

export default defineConfig({
  plugins: [tailwindcss(), svelte(), keepGitkeep()],
  // Component tests mount Svelte in jsdom, which needs Svelte's client build.
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
  // Vitest runs only the unit and component tests; e2e/ is Playwright's (`make e2e`).
  test: {
    include: ['src/**/*.test.ts'],
  },
  build: {
    outDir,
    emptyOutDir: true,
    // The rich text editor (ADR 0009) is its own chunk: it changes rarely, and keeping it apart
    // leaves the size warning meaningful for the app's own code.
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [{ name: 'richtext', test: /node_modules[\\/](@tiptap|prosemirror-|marked|linkifyjs|orderedmap|rope-sequence|w3c-keyname)/ }],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': `http://127.0.0.1:${apiPort}`,
    },
  },
})
