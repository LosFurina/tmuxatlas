import { Button, EmptyState, Skeleton } from './ui'

export interface LoadingStateProps {
  label?: string
}

/**
 * Lightweight first-paint placeholder while the authentication state is being
 * resolved. The skeletons are decorative; the labelled status is the single
 * announcement exposed to assistive technology.
 */
export function AuthLoadingState({
  label = 'Checking authentication',
}: LoadingStateProps) {
  return (
    <main
      className="app-shell flex items-center justify-center bg-background p-4"
      aria-busy="true"
    >
      <section
        className="w-full max-w-md rounded-lg border border-border bg-card p-5 shadow-sm"
        aria-label="TmuxAtlas sign-in"
      >
        <Skeleton label={label} className="mx-auto h-7 w-36" />
        <div className="mt-3 space-y-2" aria-hidden="true">
          <Skeleton className="mx-auto h-3 w-56 max-w-full" />
          <Skeleton className="mx-auto h-3 w-40 max-w-full" />
        </div>
        <div className="mt-8 space-y-4" aria-hidden="true">
          <Skeleton className="h-5 w-52 max-w-full" />
          <Skeleton className="h-3 w-full" />
          <Skeleton className="h-3 w-5/6" />
          <Skeleton className="h-10 w-full" />
        </div>
      </section>
    </main>
  )
}

/**
 * Shell-shaped placeholder used while the revisioned Workspace projection is
 * loading. It mirrors the desktop hierarchy without loading terminal code.
 */
export function WorkspaceLoadingState({
  label = 'Loading workspace',
}: LoadingStateProps) {
  return (
    <main
      className="app-shell flex min-h-0 flex-col bg-background p-3 sm:p-4"
      aria-busy="true"
    >
      <Skeleton label={label} className="h-11 w-full shrink-0" />
      <div className="mt-3 flex min-h-0 min-w-0 flex-1 gap-3" aria-hidden="true">
        <aside className="hidden w-64 shrink-0 rounded-lg border border-border bg-card p-3 md:block">
          <Skeleton className="h-8 w-full" />
          <div className="mt-5 space-y-3">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-11/12" />
            <Skeleton className="h-4 w-20" />
            <Skeleton className="h-9 w-full" />
          </div>
        </aside>
        <section className="flex min-w-0 flex-1 flex-col rounded-lg border border-border bg-card p-3 sm:p-4">
          <div className="flex items-center gap-3">
            <Skeleton className="h-7 w-40 max-w-[55%]" />
            <Skeleton className="ml-auto h-9 w-24" />
          </div>
          <Skeleton className="mt-4 min-h-0 w-full flex-1" />
        </section>
      </div>
    </main>
  )
}

export interface EmptyWorkspaceStateProps {
  onNewSession: () => void
  onOpenSetup: () => void
}

/**
 * The true, ready-but-empty Workspace state. Both recovery paths are required
 * props so a caller cannot accidentally render a dead end.
 */
export function EmptyWorkspaceState({
  onNewSession,
  onOpenSetup,
}: EmptyWorkspaceStateProps) {
  return (
    <main className="flex min-h-0 w-full flex-1 items-center justify-center bg-background p-4">
      <EmptyState
        title="No tmux sessions yet"
        description="Create a session now, or open Setup to connect and configure this host."
        action={(
          <div className="flex w-full max-w-sm flex-col justify-center gap-2 sm:w-auto sm:flex-row">
            <Button variant="primary" size="lg" onClick={onNewSession}>
              New Session
            </Button>
            <Button variant="secondary" size="lg" onClick={onOpenSetup}>
              Setup
            </Button>
          </div>
        )}
      />
    </main>
  )
}
