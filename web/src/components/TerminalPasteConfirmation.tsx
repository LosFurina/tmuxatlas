import { Button, Dialog } from './ui'

interface TerminalPasteConfirmationProps {
  text: string
  targetLabel: string
  error?: string
  onConfirm: () => void
  onCancel: () => void
}

export function TerminalPasteConfirmation({
  text,
  targetLabel,
  error,
  onConfirm,
  onCancel,
}: TerminalPasteConfirmationProps) {
  return (
    <Dialog
      open
      role="alertdialog"
      onOpenChange={open => { if (!open) onCancel() }}
      title="Paste multiple lines?"
      description={`This will send ${text.split(/\r\n|\r|\n/).length} lines to ${targetLabel}.`}
      footer={(
        <>
          <Button variant="secondary" onClick={onCancel}>Cancel</Button>
          <Button variant="primary" onClick={onConfirm}>Paste into Terminal</Button>
        </>
      )}
    >
      <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded border border-border bg-background p-3 text-xs text-foreground">
        {text}
      </pre>
      {error && <p className="mt-2 text-xs text-destructive" role="alert">{error}</p>}
    </Dialog>
  )
}
