const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

interface InertRecord {
  count: number
  hadInert: boolean
  ariaHidden: string | null
}

const inertRecords = new Map<HTMLElement, InertRecord>()

function makeElementInert(element: HTMLElement): void {
  const record = inertRecords.get(element)
  if (record) {
    record.count += 1
    return
  }
  inertRecords.set(element, {
    count: 1,
    hadInert: Boolean(element.inert),
    ariaHidden: element.getAttribute('aria-hidden'),
  })
  element.inert = true
  element.setAttribute('aria-hidden', 'true')
}

function restoreElement(element: HTMLElement): void {
  const record = inertRecords.get(element)
  if (!record) return
  record.count -= 1
  if (record.count > 0) return
  element.inert = record.hadInert
  if (record.ariaHidden === null) element.removeAttribute('aria-hidden')
  else element.setAttribute('aria-hidden', record.ariaHidden)
  inertRecords.delete(element)
}

/**
 * Makes every sibling branch outside an overlay inert. Walking ancestor
 * branches (rather than only document.body children) also works for an
 * in-tree Drawer without making the Drawer itself inert.
 */
export function makeOutsideInert(
  protectedElement: HTMLElement,
  preserveSibling: (element: HTMLElement) => boolean = () => false,
): () => void {
  const elements: HTMLElement[] = []
  let branch: HTMLElement = protectedElement
  let parent = branch.parentElement

  while (parent) {
    for (const child of Array.from(parent.children)) {
      if (
        !(child instanceof HTMLElement)
        || child === branch
        || preserveSibling(child)
        || elements.includes(child)
      ) {
        continue
      }
      elements.push(child)
      makeElementInert(child)
    }
    if (parent === document.body) break
    branch = parent
    parent = parent.parentElement
  }

  return () => {
    for (const element of elements.reverse()) restoreElement(element)
  }
}

export function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(focusableSelector))
    .filter(element => !element.hidden && element.getAttribute('aria-hidden') !== 'true')
}
