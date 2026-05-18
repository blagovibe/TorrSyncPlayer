import { useCallback, useEffect, useRef } from "react";
import "./Modal.css";

interface ModalProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({
  isOpen,
  title,
  message,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  onConfirm,
  onCancel,
}: ModalProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    if (isOpen) {
      if (!dialog.open) dialog.showModal();
    } else {
      if (dialog.open) dialog.close();
    }
  }, [isOpen]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
      }
    },
    [onCancel],
  );

  if (!isOpen) return null;

  return (
    <dialog
      ref={dialogRef}
      className="modal-overlay"
      onKeyDown={handleKeyDown}
      onClick={(e) => {
        if (e.target === dialogRef.current) onCancel();
      }}
    >
      <div className="modal-content" role="alertdialog" aria-modal="true" aria-labelledby="modal-title" aria-describedby="modal-message">
        <h3 id="modal-title" className="modal-title">{title}</h3>
        <p id="modal-message" className="modal-message">{message}</p>
        <div className="modal-actions">
          <button type="button" className="secondary-btn" onClick={onCancel}>
            {cancelLabel}
          </button>
          <button
            type="button"
            className={danger ? "danger-btn" : "primary-btn"}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </dialog>
  );
}

export default ConfirmModal;
