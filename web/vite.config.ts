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
  build: {
    outDir,
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': `http://127.0.0.1:${apiPort}`,
    },
  },
})
