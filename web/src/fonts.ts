import '@fontsource/vt323/latin-400.css'
import '@fontsource/jetbrains-mono/latin-400.css'
import '@fontsource/jetbrains-mono/latin-700.css'

const bundledTerminalFonts: Record<string, () => Promise<unknown>> = {
  'Space Mono': async () => {
    await Promise.all([import('@fontsource/space-mono/latin-400.css'), import('@fontsource/space-mono/latin-700.css')])
  },
  'JetBrains Mono': async () => {},
  'Fira Code': async () => {
    await Promise.all([import('@fontsource/fira-code/latin-400.css'), import('@fontsource/fira-code/latin-700.css')])
  },
}

export const bundledFontNames = Object.keys(bundledTerminalFonts)

export async function ensureTerminalFont(name: string): Promise<boolean> {
  const loader = bundledTerminalFonts[name]
  if (!loader) return false
  await loader()
  await document.fonts?.load(`13px "${name}"`)
  return true
}
