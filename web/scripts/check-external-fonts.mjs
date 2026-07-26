import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

const roots = ['src', '../pkg/server/dist']
const forbidden = ['fonts.googleapis.com', 'fonts.gstatic.com']
function walk(path) {
  for (const name of readdirSync(path)) {
    const child = join(path, name)
    if (statSync(child).isDirectory()) walk(child)
    else {
      const content = readFileSync(child)
      for (const host of forbidden) {
        if (content.includes(Buffer.from(host))) {
          console.error(`${child} contains forbidden runtime font host ${host}`)
          process.exitCode = 1
        }
      }
    }
  }
}
roots.forEach(walk)
