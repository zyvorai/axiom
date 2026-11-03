import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { SHORTCUT_UI } from "@/constants/testIds";
import { SHORTCUTS, SHORTCUTS_EVENT } from "@/lib/shortcuts";

// The `?` help dialog. Rows come from lib/shortcuts.js, the same list the key handler uses.
export default function ShortcutsDialog() {
    const [open, setOpen] = useState(false);

    useEffect(() => {
        const show = () => setOpen(true);
        window.addEventListener(SHORTCUTS_EVENT, show);
        return () => window.removeEventListener(SHORTCUTS_EVENT, show);
    }, []);

    const groups = [...new Set(SHORTCUTS.map((s) => s.group))];
    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogContent className="max-w-md" data-testid={SHORTCUT_UI.dialog}>
                <DialogHeader>
                    <DialogTitle>Keyboard shortcuts</DialogTitle>
                    <DialogDescription>Shortcuts are paused while you are typing in a field.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4">
                    {groups.map((g) => (
                        <section key={g} aria-label={g}>
                            <h3 className="mb-1.5 text-[11.5px] font-medium uppercase tracking-wide text-ink-3">{g}</h3>
                            <ul className="space-y-1">
                                {SHORTCUTS.filter((s) => s.group === g).map((s) => (
                                    <li key={s.label} className="flex items-center justify-between gap-4 text-[13px]">
                                        <span>{s.label}</span>
                                        <span className="flex shrink-0 items-center gap-1">
                                            {s.keys.map((k, i) => (
                                                <span key={k} className="flex items-center gap-1">
                                                    {i > 0 && s.group !== "General" && <span className="text-ink-3">then</span>}
                                                    {i > 0 && s.group === "General" && <span className="text-ink-3">or</span>}
                                                    <kbd className="mono rounded-md border border-border px-1.5 py-0.5 text-[11.5px]">{k}</kbd>
                                                </span>
                                            ))}
                                        </span>
                                    </li>
                                ))}
                            </ul>
                        </section>
                    ))}
                </div>
            </DialogContent>
        </Dialog>
    );
}
