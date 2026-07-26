import { StrictMode, type PropsWithChildren, type ReactElement } from 'react'
import { render, renderHook, type RenderHookOptions, type RenderOptions } from '@testing-library/react'

function StrictModeWrapper({ children }: PropsWithChildren) {
  return <StrictMode>{children}</StrictMode>
}

export function renderStrict(ui: ReactElement, options?: RenderOptions) {
  return render(ui, { wrapper: StrictModeWrapper, ...options })
}

export function renderHookStrict<Result, Props>(
  callback: (props: Props) => Result,
  options?: RenderHookOptions<Props>,
) {
  return renderHook(callback, { wrapper: StrictModeWrapper, ...options })
}
