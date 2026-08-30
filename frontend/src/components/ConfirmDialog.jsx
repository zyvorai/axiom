import { createContext, useCallback, useContext, useRef, useState } from "react";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";

const ConfirmContext = createContext(null);

// A single shared confirmation dialog for every destructive action in the
// app, so a misclick on a trash icon can't silently delete a production
// Secret/Namespace/Workload. Always names the specific consequence in
// `description` — never a bare "Are you sure?" — matching how a careful
// product should talk about something that can't be undone.
export function ConfirmProvider({ children }) {
    const [state, setState] = useState(null);
    const resolveRef = useRef(null);

    const confirm = useCallback((opts) => {
        return new Promise((resolve) => {
            resolveRef.current = resolve;
            setState({
                title: opts.title || "Are you sure?",
                description: opts.description || "This action cannot be undone.",
                confirmLabel: opts.confirmLabel || "Delete",
                cancelLabel: opts.cancelLabel || "Cancel",
                danger: opts.danger !== false,
            });
        });
    }, []);

    const settle = (value) => {
        resolveRef.current?.(value);
        resolveRef.current = null;
        setState(null);
    };

    return (
        <ConfirmContext.Provider value={confirm}>
            {children}
            <AlertDialog open={!!state} onOpenChange={(open) => !open && settle(false)}>
                <AlertDialogContent className="glass border-white/10">
                    <AlertDialogHeader>
                        <AlertDialogTitle>{state?.title}</AlertDialogTitle>
                        <AlertDialogDescription>{state?.description}</AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel onClick={() => settle(false)}>
                            {state?.cancelLabel}
                        </AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => settle(true)}
                            className={state?.danger ? "bg-destructive text-destructive-foreground hover:bg-destructive/90" : undefined}
                        >
                            {state?.confirmLabel}
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </ConfirmContext.Provider>
    );
}

// Returns confirm(opts) => Promise<boolean>. opts: { title, description,
// confirmLabel, cancelLabel, danger }. Resolves true only if the user
// clicked the (by default destructive-styled) confirm action.
export function useConfirm() {
    const ctx = useContext(ConfirmContext);
    if (!ctx) {
        throw new Error("useConfirm must be used within a ConfirmProvider");
    }
    return ctx;
}
