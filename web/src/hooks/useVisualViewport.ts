import { useEffect } from 'react'

const viewportProperties = [
  '--visual-viewport-height',
  '--visual-viewport-width',
  '--visual-viewport-offset-top',
  '--visual-viewport-offset-left',
] as const

function px(value: number): string {
  return `${Math.max(0, Math.round(value))}px`
}

/**
 * Mirrors the browser's visual viewport into CSS variables. iOS moves and
 * shrinks this viewport while its software keyboard is visible, while the
 * layout viewport (and 100vh) can remain unchanged.
 */
export function useVisualViewportVariables(): void {
  useEffect(() => {
    const root = document.documentElement
    const previous = new Map(viewportProperties.map(property => [
      property,
      root.style.getPropertyValue(property),
    ]))
    const previousConstrained = root.getAttribute('data-visual-viewport-constrained')
    const previousKeyboard = root.getAttribute('data-visual-keyboard-open')
    let frame: number | null = null

    const update = () => {
      frame = null
      const viewport = window.visualViewport
      const height = viewport?.height ?? window.innerHeight
      const width = viewport?.width ?? window.innerWidth
      const offsetTop = viewport?.offsetTop ?? 0
      const offsetLeft = viewport?.offsetLeft ?? 0
      root.style.setProperty(
        '--visual-viewport-height',
        px(height),
      )
      root.style.setProperty(
        '--visual-viewport-width',
        px(width),
      )
      root.style.setProperty(
        '--visual-viewport-offset-top',
        px(offsetTop),
      )
      root.style.setProperty(
        '--visual-viewport-offset-left',
        px(offsetLeft),
      )
      root.toggleAttribute('data-visual-viewport-constrained', height < 420)
      root.toggleAttribute(
        'data-visual-keyboard-open',
        Boolean(viewport && height + offsetTop <= window.innerHeight - 64),
      )
    }

    const schedule = () => {
      if (frame !== null) return
      frame = window.requestAnimationFrame(update)
    }

    update()
    const viewport = window.visualViewport
    viewport?.addEventListener('resize', schedule)
    viewport?.addEventListener('scroll', schedule)
    window.addEventListener('resize', schedule)
    window.addEventListener('orientationchange', schedule)

    return () => {
      viewport?.removeEventListener('resize', schedule)
      viewport?.removeEventListener('scroll', schedule)
      window.removeEventListener('resize', schedule)
      window.removeEventListener('orientationchange', schedule)
      if (frame !== null) window.cancelAnimationFrame(frame)
      for (const property of viewportProperties) {
        const value = previous.get(property)
        if (value) root.style.setProperty(property, value)
        else root.style.removeProperty(property)
      }
      if (previousConstrained === null) root.removeAttribute('data-visual-viewport-constrained')
      else root.setAttribute('data-visual-viewport-constrained', previousConstrained)
      if (previousKeyboard === null) root.removeAttribute('data-visual-keyboard-open')
      else root.setAttribute('data-visual-keyboard-open', previousKeyboard)
    }
  }, [])
}
