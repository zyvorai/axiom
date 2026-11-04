import { useEffect, useMemo, useState } from "react";
import { Eye, Plus, Save, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { TRIGGERS, newTrigger } from "@/components/AutoscalingCreate";
import { AUTOSCALE_EDIT as E } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";

/**
 * Shared shell for editing an autoscaler in place: load the typed fields, change them,
 * preview the structured diff, then save. A stale edit is refused by the server.
 */
function EditShell({ kind, route, target, title, onClose, onSaved, children, body, setForm, form, obj }) {
    const [preview, setPreview] = useState(null);
    const [previewFor, setPreviewFor] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState("");
    const key = body ? JSON.stringify(body) : "";
    const current = preview && previewFor === key;
    const stale = /changed since you opened/i.test(error);

    useEffect(() => {
        setPreview(null);
        setError("");
    }, [target]);

    const doPreview = async () => {
        setBusy("preview");
        setError("");
        try {
            const { data } = await api.post(`/${route}/${target.namespace}/${encodeURIComponent(target.name)}/edit/preview`, body);
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
            await api.put(`/${route}/${target.namespace}/${encodeURIComponent(target.name)}`, body);
            toast.success(`Updated ${target.name}`);
            onSaved();
            onClose();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    return (
        <Dialog open={!!target} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-2xl" data-testid={E.dialog}>
                <DialogHeader>
                    <DialogTitle>{title}</DialogTitle>
                    <DialogDescription>
                        {kind} · {target?.namespace}. Only the fields below change; everything else on the object is left alone.
                    </DialogDescription>
                </DialogHeader>
                {!obj && !error ? <Skeleton className="h-32 rounded-xl bg-muted" /> : null}
                {error ? (
                    <div className="notice danger" role="alert" data-testid={E.error}>
                        {error}
                        {stale ? " Close this dialog and open it again to start from the current version." : ""}
                    </div>
                ) : null}
                {obj && !obj.editable && (
                    <div className="notice" data-testid={E.notEditable}>
                        This object uses settings the form cannot represent, so it is read-only here:
                        <ul className="mt-1 list-disc pl-5" data-testid={E.reasons}>
                            {(obj.reasons || []).map((r) => (
                                <li key={r}>{r}</li>
                            ))}
                        </ul>
                    </div>
                )}
                {obj?.editable && form && (
                    <div tabIndex={0} role="region" aria-label="Settings" className="max-h-[55vh] space-y-3 overflow-y-auto pr-1">
                        {children}
                    </div>
                )}
                {preview && (
                    <div className="space-y-2">
                        {preview.noop ? (
                            <p className="text-sm text-ink-3" data-testid={E.noop}>Nothing would change.</p>
                        ) : (
                            <ul className="divide-y divide-border rounded-xl border border-border" data-testid={E.changes}>
                                {preview.changes.map((c) => (
                                    <li key={`${c.object}/${c.field}`} className="px-3 py-2 text-sm">
                                        <span className="mono text-[12px] text-ink-3">{c.object}</span> <b>{c.field}</b>
                                        <div className="mono text-[12px]">{String(c.from ?? "(none)")} → {String(c.to ?? "(none)")}</div>
                                    </li>
                                ))}
                            </ul>
                        )}
                        {(preview.warnings || []).map((w) => (
                            <p key={w} className="text-[11.5px] text-tone-amber">{w}</p>
                        ))}
                        {!current && <p className="text-[11.5px] text-ink-3">You changed a field after previewing — preview again.</p>}
                    </div>
                )}
                <DialogFooter className="gap-2">
                    <button type="button" className="btn btn-line" data-testid={E.preview} onClick={doPreview} disabled={!!busy || !obj?.editable}>
                        <Eye size={14} /> {busy === "preview" ? "Comparing…" : "Preview changes"}
                    </button>
                    <button type="button" className="btn btn-fill" data-testid={E.save} onClick={save} disabled={!!busy || !current || preview?.noop}>
                        <Save size={14} /> {busy === "save" ? "Saving…" : "Save"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function useEditable(route, target) {
    const [obj, setObj] = useState(null);
    const [form, setForm] = useState(null);
    const [error, setError] = useState("");
    useEffect(() => {
        setObj(null);
        setForm(null);
        setError("");
        if (!target) return undefined;
        let alive = true;
        api
            .get(`/${route}/${target.namespace}/${encodeURIComponent(target.name)}/edit`)
            .then((r) => {
                if (alive) {
                    setObj(r.data);
                    setForm(r.data);
                }
            })
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [route, target]);
    return { obj, form, setForm, error };
}

// ---------------------------------------------------------------- VPA
export function VpaEditDialog({ target, onClose, onSaved }) {
    const { obj, form, setForm, error } = useEditable("vpas", target);
    const body = useMemo(() => {
        if (!form) return null;
        const b = { updateMode: form.updateMode, controlledResources: form.controlledResources, resourceVersion: form.resourceVersion };
        if (form.containers) b.containers = form.containers;
        return b;
    }, [form]);
    const has = (r) => (form?.controlledResources || []).includes(r);
    const toggle = (r, on) => setForm((f) => ({ ...f, controlledResources: on ? [...new Set([...(f.controlledResources || []), r])] : (f.controlledResources || []).filter((x) => x !== r) }));

    return (
        <EditShell kind="VerticalPodAutoscaler" route="vpas" target={target} title={`Edit ${target?.name || ""}`} onClose={onClose} onSaved={onSaved} body={body} form={form} setForm={setForm} obj={obj || (error ? { editable: false, reasons: [error] } : null)}>
            <div className="space-y-1.5">
                <Label htmlFor="vpa-edit-mode" className="text-xs text-muted-foreground">Update mode</Label>
                <select id="vpa-edit-mode" data-testid={E.mode} className="select w-full" value={form?.updateMode || "Off"} onChange={(e) => setForm((f) => ({ ...f, updateMode: e.target.value }))}>
                    <option value="Off">Off — recommend only</option>
                    <option value="Initial">Initial — apply to new pods</option>
                    <option value="Recreate">Recreate — restart pods to apply</option>
                    <option value="Auto">Auto</option>
                </select>
            </div>
            <div className="flex items-center gap-5 text-sm">
                <label className="flex items-center gap-2"><input type="checkbox" data-testid={E.cpu} checked={has("cpu")} onChange={(e) => toggle("cpu", e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" /> CPU</label>
                <label className="flex items-center gap-2"><input type="checkbox" data-testid={E.memory} checked={has("memory")} onChange={(e) => toggle("memory", e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" /> Memory</label>
            </div>
        </EditShell>
    );
}

// ---------------------------------------------------------------- KEDA
export function KedaEditDialog({ target, onClose, onSaved }) {
    const { t } = useTranslation();
    const { obj, form, setForm, error } = useEditable("scaledobjects", target);
    const [auths, setAuths] = useState([]);
    useEffect(() => {
        if (!target) return;
        api.get("/triggerauthentications", { params: { namespace: target.namespace } }).then((r) => setAuths(r.data?.items || [])).catch(() => setAuths([]));
    }, [target]);

    const body = useMemo(() => {
        if (!form) return null;
        const b = {
            minReplicas: Number(form.minReplicas),
            maxReplicas: Number(form.maxReplicas),
            resourceVersion: form.resourceVersion,
            triggers: (form.triggers || []).map((trg) => {
                const settings = Object.fromEntries(Object.entries(trg.settings || {}).filter(([, v]) => v !== "" && v != null));
                const out = { type: trg.type, settings };
                if (trg.name) out.name = trg.name;
                if (trg.authenticationRef) out.authenticationRef = trg.authenticationRef;
                return out;
            }),
        };
        if (form.pollingInterval) b.pollingInterval = Number(form.pollingInterval);
        if (form.cooldownPeriod) b.cooldownPeriod = Number(form.cooldownPeriod);
        return b;
    }, [form]);
    const setTrigger = (i, patch) => setForm((f) => ({ ...f, triggers: f.triggers.map((x, j) => (j === i ? { ...x, ...patch } : x)) }));

    return (
        <EditShell kind="ScaledObject" route="scaledobjects" target={target} title={`Edit ${target?.name || ""}`} onClose={onClose} onSaved={onSaved} body={body} form={form} setForm={setForm} obj={obj || (error ? { editable: false, reasons: [error] } : null)}>
            <div className="grid gap-3 sm:grid-cols-4">
                {[["minReplicas", "Min replicas", E.min, 0], ["maxReplicas", "Max replicas", E.max, 1], ["pollingInterval", "Polling (s)", E.polling, 1], ["cooldownPeriod", "Cooldown (s)", E.cooldown, 1]].map(([k, l, id, min]) => (
                    <div key={k} className="space-y-1.5">
                        <Label htmlFor={`keda-edit-${k}`} className="text-xs text-muted-foreground">{l}</Label>
                        <Input id={`keda-edit-${k}`} data-testid={id} type="number" min={min} value={form?.[k] ?? ""} onChange={(e) => setForm((f) => ({ ...f, [k]: e.target.value }))} className="bg-muted/30" />
                    </div>
                ))}
            </div>
            {(form?.triggers || []).map((trg, i) => {
                const def = TRIGGERS[trg.type];
                if (!def) return null;
                return (
                    <fieldset key={i} className="space-y-2 rounded-xl border border-border p-3">
                        <legend className="px-1 text-xs text-muted-foreground">Trigger {i + 1} · {t(def.labelKey)}</legend>
                        <div className="grid gap-2 sm:grid-cols-2">
                            {def.fields.map((f) => (
                                <div key={f.k} className="space-y-1">
                                    <Label htmlFor={`keda-edit-${i}-${f.k}`} className="text-xs text-muted-foreground">{t(f.lKey)}</Label>
                                    {f.options ? (
                                        <select id={`keda-edit-${i}-${f.k}`} data-testid={E.triggerField(i, f.k)} className="select w-full" value={trg.settings?.[f.k] || f.options[0]} onChange={(e) => setTrigger(i, { settings: { ...trg.settings, [f.k]: e.target.value } })}>
                                            {f.options.map((o) => (
                                                <option key={o} value={o}>{o}</option>
                                            ))}
                                        </select>
                                    ) : (
                                        <Input id={`keda-edit-${i}-${f.k}`} data-testid={E.triggerField(i, f.k)} value={trg.settings?.[f.k] || ""} onChange={(e) => setTrigger(i, { settings: { ...trg.settings, [f.k]: e.target.value } })} placeholder={f.ph} className="bg-muted/30" />
                                    )}
                                </div>
                            ))}
                        </div>
                        {def.auth && (
                            <select aria-label="TriggerAuthentication" className="select w-full" value={trg.authenticationRef || ""} onChange={(e) => setTrigger(i, { authenticationRef: e.target.value })}>
                                <option value="">No credentials</option>
                                {auths.map((a) => (
                                    <option key={a.name} value={a.name}>{a.name}</option>
                                ))}
                            </select>
                        )}
                        {(form.triggers || []).length > 1 && (
                            <button type="button" className="btn btn-danger btn-sm" data-testid={E.triggerRemove(i)} aria-label={`Remove trigger ${i + 1}`} onClick={() => setForm((f) => ({ ...f, triggers: f.triggers.filter((_, j) => j !== i) }))}>
                                <Trash2 size={13} />
                            </button>
                        )}
                    </fieldset>
                );
            })}
            <button type="button" className="btn btn-line btn-sm" data-testid={E.addTrigger} onClick={() => setForm((f) => ({ ...f, triggers: [...(f.triggers || []), newTrigger("cpu")] }))}>
                <Plus size={13} /> Add trigger
            </button>
        </EditShell>
    );
}
