import { useCallback, useRef } from "react";

/**
 * Radix returns focus to a <DialogTrigger> on close, but most dialogs in this app are opened
 * from ordinary buttons via state (`open={…}`), so focus fell to <body> and keyboard users lost
 * their place.
 *
 * NOTE: our DialogContent wrappers mount at page load (only Radix's inner content is
 * conditional), so we must capture the trigger when the dialog actually OPENS —
 * `onOpenAutoFocus` fires before Radix moves focus inside — not on first render.
 * On close we put focus back, unless that element is gone (e.g. a menu item that
 * unmounted), in which case Radix's default applies.
 */
export function useReturnFocus({ onOpenAutoFocus, onCloseAutoFocus } = {}) {
    const trigger = useRef(null);
    const onOpen = useCallback(
        (event) => {
            trigger.current = document.activeElement;
            onOpenAutoFocus?.(event);
        },
        [onOpenAutoFocus]
    );
    const onClose = useCallback(
        (event) => {
            onCloseAutoFocus?.(event);
            const el = trigger.current;
            trigger.current = null;
            if (event.defaultPrevented) return;
            if (el && el !== document.body && el.isConnected) {
                event.preventDefault();
                el.focus();
            }
        },
        [onCloseAutoFocus]
    );
    return { onOpenAutoFocus: onOpen, onCloseAutoFocus: onClose };
}
