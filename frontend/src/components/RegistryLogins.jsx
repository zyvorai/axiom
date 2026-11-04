import { useCallback, useEffect, useState } from "react";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useConfirm } from "@/components/ConfirmDialog";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { REGCRED as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";
const BLANK = { name: "", registry: "", username: "", password: "" };

/** Logins for private OCI chart registries. Write-only: the password is never shown again. */
export default function RegistryLogins({ open, onOpenChange, onChanged }) {
    const confirm = useConfirm();
    const [rows, setRows] = useState([]);
    const [form, setForm] = useState(BLANK);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const load = useCallback(async () => {
        try {
            const { data } = await api.get("/helm/registry-credentials");
            setRows(data || []);
        } catch (e) {
            setError(errText(e));
        }
    }, []);

    useEffect(() => {
        if (open) {
            setForm(BLANK);
            setError("");
            load();
        }
    }, [open, load]);

    const save = async () => {
        setBusy(true);
        setError("");
        try {
            await api.post("/helm/registry-credentials", { name: form.name.trim(), registry: form.registry.trim(), username: form.username, password: form.password });
            toast.success(`Saved login ${form.name}`, { description: "The password is write-only and cannot be shown again." });
            setForm(BLANK);
            await load();
            onChanged?.();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    const remove = async (r) => {
        const ok = await confirm({ title: `Delete the login ${r.name}?`, description: "Repositories that use it must be removed or changed first.", confirmLabel: "Delete" });
        if (!ok) return;
        try {
            await api.delete(`/helm/registry-credentials/${encodeURIComponent(r.name)}`);
            toast.success(`Deleted ${r.name}`);
            await load();
            onChanged?.();
        } catch (e) {
            toast.error("Delete failed", { description: errText(e) });
        }
    };

    const filled = form.name && form.registry && form.username && form.password;
    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>Registry logins</DialogTitle>
                    <DialogDescription>
                        For private OCI chart registries. A login is sent only to the registry it is for, and never shown again once saved.
                    </DialogDescription>
                </DialogHeader>
                <div tabIndex={0} role="region" aria-label="Registry logins" className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
                    {rows.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No registry logins yet.</p>
                    ) : (
                        <ul className="divide-y divide-border rounded-xl border border-border">
                            {rows.map((r) => (
                                <li key={r.name} className="flex items-center justify-between gap-3 px-3 py-2" data-testid={T.row(r.name)}>
                                    <div className="min-w-0">
                                        <div className="text-sm font-medium">{r.name}</div>
                                        <div className="mono truncate text-[11.5px] text-ink-3">{r.username}@{r.registry}{(r.inUse || []).length ? ` · used by ${r.inUse.join(", ")}` : ""}</div>
                                    </div>
                                    <button type="button" className="btn btn-danger btn-sm" data-testid={T.remove(r.name)} aria-label={`Delete login ${r.name}`} onClick={() => remove(r)}>
                                        <Trash2 size={13} />
                                    </button>
                                </li>
                            ))}
                        </ul>
                    )}
                    <form
                        className="space-y-3 rounded-xl border border-border p-3"
                        onSubmit={(e) => {
                            e.preventDefault();
                            if (filled) save();
                        }}
                    >
                        <div className="text-sm font-medium">Add a login</div>
                        <p className="text-[11.5px] text-muted-foreground" data-testid={T.writeOnly}>
                            Write-only: to change a password, delete the login and add it again.
                        </p>
                        <div className="grid gap-3 sm:grid-cols-2">
                            <div className="space-y-1.5">
                                <Label htmlFor="regcred-name" className="text-xs text-muted-foreground">Name</Label>
                                <Input id="regcred-name" data-testid={T.name} value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="ghcr-private" className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="regcred-registry" className="text-xs text-muted-foreground">Registry host</Label>
                                <Input id="regcred-registry" data-testid={T.registry} value={form.registry} onChange={(e) => set({ registry: e.target.value })} placeholder="ghcr.io" className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="regcred-user" className="text-xs text-muted-foreground">Username</Label>
                                <Input id="regcred-user" data-testid={T.username} autoComplete="off" value={form.username} onChange={(e) => set({ username: e.target.value })} className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="regcred-pass" className="text-xs text-muted-foreground">Password or token</Label>
                                <Input id="regcred-pass" data-testid={T.password} type="password" autoComplete="new-password" value={form.password} onChange={(e) => set({ password: e.target.value })} className="bg-muted/30" />
                            </div>
                        </div>
                        {error ? <div className="notice danger" role="alert" data-testid={T.error}>{error}</div> : null}
                        <button type="submit" className="btn btn-fill btn-sm" data-testid={T.save} disabled={busy || !filled}>
                            <Plus size={13} /> {busy ? "Saving…" : "Save login"}
                        </button>
                    </form>
                </div>
                <DialogFooter />
            </DialogContent>
        </Dialog>
    );
}

export function RegistryLoginsButton({ onClick }) {
    return (
        <button type="button" className="btn btn-line btn-sm" data-testid={T.manage} onClick={onClick}>
            <KeyRound size={13} /> Registry logins
        </button>
    );
}
