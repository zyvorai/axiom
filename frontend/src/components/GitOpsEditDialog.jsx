import { useEffect, useMemo, useState } from "react";
import { Eye, Save } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import MacTerm from "@/components/MacTerm";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { GITOPS_EDIT as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:gitops.generic");

function Field({ id, label, testId, value, onChange, placeholder, disabled }) {
    return (
        <div className="space-y-1.5">
            <Label htmlFor={id} className="text-xs text-muted-foreground">{label}</Label>
            <Input id={id} data-testid={testId} value={value ?? ""} disabled={disabled} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="bg-muted/30" />
        </div>
    );
}

// The typed body the server expects: only the managed fields, plus the version we read.
function toBody(f) {
    const body = { source: { ...f.source }, destination: { ...f.destination }, resourceVersion: f.resourceVersion };
    if (f.project) body.project = f.project;
    if (f.interval) body.interval = f.interval;
    if (f.values) body.values = f.values;
    if (f.syncPolicy) body.syncPolicy = { ...f.syncPolicy };
    return body;
}

/** Edit an application's managed fields in place: review the structured change, then save. */
export default function GitOpsEditDialog({ app, onClose, onSaved }) {
    const { t } = useTranslation();
    const [obj, setObj] = useState(null);
    const [form, setForm] = useState(null);
    const [preview, setPreview] = useState(null);
    const [previewFor, setPreviewFor] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState("");

    useEffect(() => {
        setObj(null);
        setForm(null);
        setPreview(null);
        setError("");
        if (!app) return undefined;
        let alive = true;
        api
            .get(`/gitops/${app.kind}/${app.namespace}/${encodeURIComponent(app.name)}/edit`)
            .then((r) => {
                if (!alive) return;
                setObj(r.data);
                setForm(r.data);
            })
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [app]);

    const body = useMemo(() => (form ? toBody(form) : null), [form]);
    const key = body ? JSON.stringify(body) : "";
    const current = preview && previewFor === key;
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));
    const setSource = (patch) => setForm((f) => ({ ...f, source: { ...f.source, ...patch } }));
    const setSync = (patch) => setForm((f) => ({ ...f, syncPolicy: { ...(f.syncPolicy || {}), ...patch } }));

    const doPreview = async () => {
        setBusy("preview");
        setError("");
        try {
            const { data } = await api.post(`/gitops/${app.kind}/${app.namespace}/${encodeURIComponent(app.name)}/edit/preview`, body);
            setPreview(data);
            setPreviewFor(key);
        } catch (e) {
            setPreview(null);
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    const save = async () => {
        setBusy("save");
        setError("");
        try {
            await api.put(`/gitops/${app.kind}/${app.namespace}/${encodeURIComponent(app.name)}`, body);
            toast.success(t("toasts:gitopsEdit.updated", { name: app.name }));
            onSaved();
            onClose();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    if (!app) return null;
    const helmSource = !!(form?.source?.chart);
    const flux = form && form.engine !== "argocd";
    const stale = /changed since you opened/i.test(error);
    return (
        <Dialog open={!!app} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-2xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:gitopsEdit.title", { name: app.name })}</DialogTitle>
                    <DialogDescription>
                        {t("dialogs:gitopsEdit.description", { kind: app.kind, namespace: app.namespace })}
                    </DialogDescription>
                </DialogHeader>

                {!obj && !error ? <Skeleton className="h-32 rounded-xl bg-muted" /> : null}
                {error ? (
                    <div className="notice danger" role="alert" data-testid={stale ? T.stale : T.error}>
                        {error}
                        {stale ? t("dialogs:gitopsEdit.staleHint") : ""}
                    </div>
                ) : null}

                {obj && !obj.editable && (
                    <div className="notice" data-testid={T.notEditable}>
                        {t("dialogs:gitopsEdit.notEditable")}
                        <ul className="mt-1 list-disc pl-5" data-testid={T.reasons}>
                            {(obj.reasons || []).map((r) => (
                                <li key={r}>{r}</li>
                            ))}
                        </ul>
                    </div>
                )}

                {obj?.editable && form && (
                    <div tabIndex={0} role="region" aria-label={t("dialogs:gitopsEdit.region")} className="max-h-[55vh] space-y-3 overflow-y-auto pr-1">
                        <div className="grid gap-3 sm:grid-cols-2">
                            {helmSource ? (
                                <Field id="gitops-edit-chart-version" label={t("dialogs:gitops.create.chartVersion")} testId={T.chartVersion} value={form.source.chartVersion} onChange={(v) => setSource({ chartVersion: v })} />
                            ) : (
                                <>
                                    <Field id="gitops-edit-revision" label={t("dialogs:gitops.create.revision")} testId={T.revision} value={form.source.revision} onChange={(v) => setSource({ revision: v })} />
                                    <Field id="gitops-edit-path" label={t("dialogs:gitops.create.path")} testId={T.path} value={form.source.path} onChange={(v) => setSource({ path: v })} />
                                </>
                            )}
                            <Field id="gitops-edit-dest-namespace" label={t("dialogs:gitops.create.destNamespace")} testId={T.destNamespace} value={form.destination?.namespace} onChange={(v) => set({ destination: { ...form.destination, namespace: v } })} />
                            {flux && <Field id="gitops-edit-interval" label={t("dialogs:gitops.create.interval")} testId={T.interval} value={form.interval} onChange={(v) => set({ interval: v })} />}
                        </div>
                        {helmSource && (
                            <div className="space-y-1.5">
                                <Label htmlFor="gitops-edit-values" className="text-xs text-muted-foreground">{t("dialogs:gitopsEdit.valuesYaml")}</Label>
                                <MacTerm title="values.yaml" bodyClassName="!p-0">
                                    <Textarea id="gitops-edit-values" data-testid={T.values} value={form.values || ""} onChange={(e) => set({ values: e.target.value })} className="macos-term-body mono min-h-[110px] resize-y rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                                </MacTerm>
                            </div>
                        )}
                        {form.syncPolicy && form.engine === "argocd" && (
                            <fieldset className="space-y-1.5 rounded-xl border border-border p-3">
                                <legend className="px-1 text-xs text-muted-foreground">{t("dialogs:gitops.create.sync")}</legend>
                                {[["automated", "dialogs:gitopsEdit.automated"], ["prune", "dialogs:gitopsEdit.prune"], ["selfHeal", "dialogs:gitopsEdit.selfHeal"]].map(([k, lKey]) => (
                                    <label key={k} className="flex cursor-pointer items-center gap-2 text-sm">
                                        <input type="checkbox" checked={!!form.syncPolicy[k]} onChange={(e) => setSync({ [k]: e.target.checked })} className="h-4 w-4 rounded border-border accent-primary" />
                                        {t(lKey)}
                                    </label>
                                ))}
                            </fieldset>
                        )}
                    </div>
                )}

                {preview && (
                    <div className="space-y-2">
                        {preview.noop ? (
                            <p className="text-sm text-ink-3" data-testid={T.noop}>{t("dialogs:gitopsEdit.noop")}</p>
                        ) : (
                            <ul className="divide-y divide-border rounded-xl border border-border" data-testid={T.changes}>
                                {preview.changes.map((c) => (
                                    <li key={`${c.object}/${c.field}`} className="px-3 py-2 text-sm">
                                        <span className="mono text-[12px] text-ink-3">{c.object}</span> <b>{c.field}</b>
                                        <div className="mono text-[12px]">{String(c.from ?? t("dialogs:gitopsEdit.none"))} → {String(c.to ?? t("dialogs:gitopsEdit.none"))}</div>
                                    </li>
                                ))}
                            </ul>
                        )}
                        {(preview.warnings || []).map((w) => (
                            <p key={w} className="text-[11.5px] text-tone-amber">{w}</p>
                        ))}
                        {!current && <p className="text-[11.5px] text-ink-3">{t("dialogs:gitopsEdit.staleChange")}</p>}
                    </div>
                )}

                <DialogFooter className="gap-2">
                    <button type="button" className="btn btn-line" data-testid={T.preview} onClick={doPreview} disabled={!!busy || !obj?.editable}>
                        <Eye size={14} /> {busy === "preview" ? t("dialogs:gitopsEdit.comparing") : t("dialogs:gitopsEdit.previewChanges")}
                    </button>
                    <button type="button" className="btn btn-fill" data-testid={T.save} onClick={save} disabled={!!busy || !current || preview?.noop}>
                        <Save size={14} /> {busy === "save" ? t("common:actions.saving") : t("common:actions.save")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
