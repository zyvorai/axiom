import { useCallback, useEffect, useState } from "react";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import { useConfirm } from "@/components/ConfirmDialog";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { CREDS as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:generic");
const BLANK = { engine: "argocd", name: "", namespace: "", url: "", authType: "token", username: "", password: "", token: "", sshPrivateKey: "", knownHosts: "" };

/** Repository credentials for Argo CD / Flux. Write-only: values are never shown again. */
export default function GitOpsCredentials({ open, onOpenChange }) {
    const confirm = useConfirm();
    const { t } = useTranslation();
    const [rows, setRows] = useState([]);
    const [form, setForm] = useState(BLANK);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const load = useCallback(async () => {
        try {
            const [a, f] = await Promise.all([api.get("/gitops/credentials", { params: { engine: "argocd" } }), api.get("/gitops/credentials", { params: { engine: "flux" } })]);
            setRows([...(a.data || []), ...(f.data || [])]);
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
            const body = { engine: form.engine, name: form.name.trim(), url: form.url.trim(), authType: form.authType, kind: "git" };
            if (form.namespace.trim()) body.namespace = form.namespace.trim();
            if (form.authType === "basic") {
                body.username = form.username;
                body.password = form.password;
            } else if (form.authType === "token") {
                body.token = form.token;
                if (form.username) body.username = form.username;
            } else {
                body.sshPrivateKey = form.sshPrivateKey;
                if (form.knownHosts) body.knownHosts = form.knownHosts;
            }
            await api.post("/gitops/credentials", body);
            toast.success(t("toasts:gitopsCredentials.saved", { name: form.name }), { description: t("toasts:gitopsCredentials.savedDescription") });
            setForm(BLANK);
            load();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    const remove = async (r) => {
        const ok = await confirm({ title: t("dialogs:gitopsCredentials.deleteTitle", { name: r.name }), description: t("dialogs:gitopsCredentials.deleteDescription"), confirmLabel: t("common:actions.delete") });
        if (!ok) return;
        try {
            await api.delete(`/gitops/credentials/${r.engine}/${encodeURIComponent(r.namespace)}/${encodeURIComponent(r.name)}`);
            toast.success(t("toasts:gitopsCredentials.deleted", { name: r.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:gitopsCredentials.deleteFailed"), { description: errText(e) });
        }
    };

    const filled = form.name && form.url && (form.authType === "token" ? form.token : form.authType === "basic" ? form.username && form.password : form.sshPrivateKey);
    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-2xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:gitopsCredentials.title")}</DialogTitle>
                    <DialogDescription>
                        {t("dialogs:gitopsCredentials.description")}
                    </DialogDescription>
                </DialogHeader>
                <div tabIndex={0} role="region" aria-label={t("dialogs:gitopsCredentials.region")} className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
                    {rows.length === 0 ? (
                        <p className="text-sm text-muted-foreground">{t("dialogs:gitopsCredentials.empty")}</p>
                    ) : (
                        <ul className="divide-y divide-border rounded-xl border border-border">
                            {rows.map((r) => (
                                <li key={`${r.engine}/${r.namespace}/${r.name}`} className="flex items-center justify-between gap-3 px-3 py-2" data-testid={T.row(r.name)}>
                                    <div className="min-w-0">
                                        <div className="text-sm font-medium">{r.name} <span className="pill soft ml-1 text-[10px]">{r.engine}</span> <span className="pill soft text-[10px]">{r.authType}</span></div>
                                        <div className="mono truncate text-[11.5px] text-ink-3">{r.url} · {r.namespace}</div>
                                    </div>
                                    {r.managedByAxiom && (
                                        <button type="button" className="btn btn-danger btn-sm" data-testid={T.remove(r.name)} aria-label={t("dialogs:gitopsCredentials.deleteAria", { name: r.name })} onClick={() => remove(r)}>
                                            <Trash2 size={13} />
                                        </button>
                                    )}
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
                        <div className="text-sm font-medium">{t("dialogs:gitopsCredentials.add")}</div>
                        <p className="text-[11.5px] text-muted-foreground" data-testid={T.writeOnly}>
                            {t("dialogs:gitopsCredentials.writeOnly")}
                        </p>
                        <div className="grid gap-3 sm:grid-cols-3">
                            <div className="space-y-1.5">
                                <Label htmlFor="cred-engine" className="text-xs text-muted-foreground">{t("dialogs:gitopsCredentials.for")}</Label>
                                <select id="cred-engine" data-testid={T.engine} className="select w-full" value={form.engine} onChange={(e) => set({ engine: e.target.value })}>
                                    <option value="argocd">Argo CD</option>
                                    <option value="flux">Flux</option>
                                </select>
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="cred-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                                <Input id="cred-name" data-testid={T.name} value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="private-repo" className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="cred-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                                <Input id="cred-namespace" data-testid={T.namespace} value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} placeholder={form.engine === "flux" ? "flux-system" : "argocd"} className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5 sm:col-span-2">
                                <Label htmlFor="cred-url" className="text-xs text-muted-foreground">{t("dialogs:gitopsCredentials.url")}</Label>
                                <Input id="cred-url" data-testid={T.url} value={form.url} onChange={(e) => set({ url: e.target.value })} placeholder="https://git.example.com/org/repo.git" className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="cred-auth" className="text-xs text-muted-foreground">{t("dialogs:gitopsCredentials.auth")}</Label>
                                <select id="cred-auth" data-testid={T.authType} className="select w-full" value={form.authType} onChange={(e) => set({ authType: e.target.value })}>
                                    <option value="token">{t("dialogs:gitopsCredentials.authToken")}</option>
                                    <option value="basic">{t("dialogs:gitopsCredentials.authBasic")}</option>
                                    <option value="ssh">{t("dialogs:gitopsCredentials.authSsh")}</option>
                                </select>
                            </div>
                        </div>
                        {form.authType === "basic" && (
                            <div className="grid gap-3 sm:grid-cols-2">
                                <div className="space-y-1.5">
                                    <Label htmlFor="cred-user" className="text-xs text-muted-foreground">{t("auth:login.username")}</Label>
                                    <Input id="cred-user" data-testid={T.username} autoComplete="off" value={form.username} onChange={(e) => set({ username: e.target.value })} className="bg-muted/30" />
                                </div>
                                <div className="space-y-1.5">
                                    <Label htmlFor="cred-pass" className="text-xs text-muted-foreground">{t("auth:login.password")}</Label>
                                    <Input id="cred-pass" data-testid={T.password} type="password" autoComplete="new-password" value={form.password} onChange={(e) => set({ password: e.target.value })} className="bg-muted/30" />
                                </div>
                            </div>
                        )}
                        {form.authType === "token" && (
                            <div className="space-y-1.5">
                                <Label htmlFor="cred-token" className="text-xs text-muted-foreground">{t("dialogs:gitopsCredentials.token")}</Label>
                                <Input id="cred-token" data-testid={T.token} type="password" autoComplete="new-password" value={form.token} onChange={(e) => set({ token: e.target.value })} className="bg-muted/30" />
                            </div>
                        )}
                        {form.authType === "ssh" && (
                            <div className="space-y-3">
                                <div className="space-y-1.5">
                                    <Label htmlFor="cred-ssh" className="text-xs text-muted-foreground">{t("dialogs:gitopsCredentials.privateKey")}</Label>
                                    <Textarea id="cred-ssh" data-testid={T.sshKey} value={form.sshPrivateKey} onChange={(e) => set({ sshPrivateKey: e.target.value })} className="mono min-h-[90px] text-xs" spellCheck={false} autoComplete="off" />
                                </div>
                                <div className="space-y-1.5">
                                    <Label htmlFor="cred-hosts" className="text-xs text-muted-foreground">{t("dialogs:gitopsCredentials.knownHosts")}</Label>
                                    <Textarea id="cred-hosts" data-testid={T.knownHosts} value={form.knownHosts} onChange={(e) => set({ knownHosts: e.target.value })} className="mono min-h-[50px] text-xs" spellCheck={false} />
                                </div>
                            </div>
                        )}
                        {error ? <div className="notice danger" role="alert" data-testid={T.error}>{error}</div> : null}
                        <button type="submit" className="btn btn-fill btn-sm" data-testid={T.save} disabled={busy || !filled}>
                            <Plus size={13} /> {busy ? t("common:actions.saving") : t("dialogs:gitopsCredentials.save")}
                        </button>
                    </form>
                </div>
                <DialogFooter />
            </DialogContent>
        </Dialog>
    );
}

export function CredentialsButton({ onClick }) {
    const { t } = useTranslation();
    return (
        <button type="button" className="btn btn-line btn-sm" data-testid={T.open} onClick={onClick}>
            <KeyRound size={14} /> {t("dialogs:gitopsCredentials.button")}
        </button>
    );
}
