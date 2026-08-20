import { Button } from '@/components/ui/button'

export interface ConfirmDialogProps {
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: 'default' | 'destructive'
  onConfirm: () => void
  onCancel: () => void
}

// In-app replacement for window.confirm(), styled to match ModalShell.
export function ConfirmDialog({
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  tone = 'default',
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={onCancel}
    >
      <div
        className="bg-background border rounded-lg shadow-lg w-full max-w-sm"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        aria-describedby="confirm-dialog-description"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-4 py-4">
          <h2 id="confirm-dialog-title" className="text-base font-semibold text-foreground">
            {title}
          </h2>
          <p id="confirm-dialog-description" className="mt-1.5 text-sm text-muted-foreground">
            {description}
          </p>
        </div>
        <div className="flex items-center justify-end gap-2 border-t px-4 py-3">
          <Button size="sm" variant="outline" onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button
            size="sm"
            variant={tone === 'destructive' ? 'destructive' : 'default'}
            onClick={onConfirm}
            autoFocus
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}
