import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { renderStrict } from '../test/render'
import { FleetHealthView } from './FleetHealth'

describe('FleetHealthView', () => {
  it('keeps duplicate display names separate and only copies remediation', async () => {
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    renderStrict(<FleetHealthView records={[
      {
        facts: { host_id: 'host-a', display_name: 'duplicate', version: 'v1' },
        summary: 'version-behind',
        reasons: [{
          code: 'version-behind', severity: 2, message: 'Behind.',
          remediation: 'tmuxatlas update',
        }],
      },
      {
        facts: { host_id: 'host-b', display_name: 'duplicate', version: 'v2' },
        summary: 'offline',
        reasons: [{ code: 'offline', severity: 4, message: 'Offline.' }],
      },
    ]} />)
    expect(screen.getAllByText('duplicate')).toHaveLength(2)
    expect(document.querySelectorAll('[data-host-id]')).toHaveLength(2)
    fireEvent.click(screen.getByLabelText('Copy remediation for host-a'))
    expect(writeText).toHaveBeenCalledWith('tmuxatlas update')
  })
})
