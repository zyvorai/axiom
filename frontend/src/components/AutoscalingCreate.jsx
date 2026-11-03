import { useEffect, useMemo, useState } from "react";
import { Eye, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import MacTerm from "@/components/MacTerm";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { VPA_CREATE as V, KEDA_CREATE as K } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:autoscalingCreate.generic");

function useWorkloads(open) {
    const [list, setList] = useState([]);
    useEffect(() => {
        if (!open) return;
        api.get("/workloads").then((r) => setList(r.data || [])).catch(() => setList([]));
    }, [open]);
    return list;
}

function Preview({ preview, yamlId, warnId, stale }) {
    const { t } = useTranslation();
    if (!preview) return null;
    return (
        <div className="space-y-2">
            {(preview.warnings || []).length > 0 && (
                <ul className="notice space-y-1" data-testid={warnId}>
                    {preview.warnings.map((w) => (
                        <li key={w}>{w}</li>
                    ))}
                </ul>
            )}
            <MacTerm title={stale ? t("dialogs:gitops.create.previewStale") : t("dialogs:gitops.create.previewTitle")} bodyClassName="!p-0">
                <pre tabIndex={0} role="region" aria-label={t("dialogs:gitops.create.generatedYaml")} className="macos-term-body mono max-h-56 overflow-auto px-4 py-3 text-xs leading-[1.55]" data-testid={yamlId}>{preview.yaml}</pre>
            </MacTerm>
        </div>
    );
}

function TargetSelect({ id, testId, workloads, value, onChange, kinds }) {
    const { t } = useTranslation();
    return (
        <div className="space-y-1.5">
            <Label htmlFor={id} className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.targetWorkload")}</Label>
            <select id={id} data-testid={testId} className="select w-full" value={value} onChange={(e) => onChange(e.target.value)}>
                <option value="">{t("dialogs:autoscaling.create.selectWorkload")}</option>
                {workloads
                    .filter((w) => !kinds || kinds.includes(w.kind))
                    .map((w) => (
                        <option key={`${w.namespace}|${w.kind}|${w.name}`} value={`${w.namespace}|${w.kind}|${w.name}`}>
                            {w.namespace}/{w.kind}/{w.name}
                        </option>
                    ))}
            </select>
        </div>
    );
}

// ---------------------------------------------------------------- VPA
export function VpaCreateDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const workloads = useWorkloads(open);
    const [target, setTarget] = useState("");
    const [name, setName] = useState("");
    const [mode, setMode] = useState("Off");
    const [cpu, setCpu] = useState(true);
    const [mem, setMem] = useState(true);
    const [preview, setPreview] = useState(null);
    const [previewFor, setPreviewFor] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState("");

    useEffect(() => {
        if (open) {
            setTarget("");
            setName("");
            setMode("Off");
            setCpu(true);
            setMem(true);
            setPreview(null);
            setError("");
        }
    }, [open]);

    const body = useMemo(() => {
        const [namespace, targetKind, targetName] = target.split("|");
        return { namespace, name: name || (targetName ? `${targetName}-vpa` : ""), targetKind, targetName, updateMode: mode, controlledResources: [cpu && "cpu", mem && "memory"].filter(Boolean) };
    }, [target, name, mode, cpu, mem]);
    const key = JSON.stringify(body);
    const current = preview && previewFor === key;

    const doPreview = async () => {
        setBusy("preview");
        setError("");
        try {
            const { data } = await api.post("/vpas/preview", body);
            setPreview(data);
            setPreviewFor(key);
        } catch (e) {
            setPreview(null);
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };
    const create = async () => {
        setBusy("create");
        setError("");
        try {
            await api.post("/vpas", body);
            toast.success(t("toasts:autoscalingCreate.created", { name: body.name }));
            onCreated();
            onOpenChange(false);
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-xl" data-testid={V.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:autoscalingCreate.vpa.title")}</DialogTitle>
                    <DialogDescription>{t("dialogs:autoscalingCreate.vpa.description")}</DialogDescription>
                </DialogHeader>
                <div tabIndex={0} role="region" aria-label={t("dialogs:autoscalingCreate.vpa.region")} className="max-h-[55vh] space-y-3 overflow-y-auto pr-1">
                    <TargetSelect id="vpa-target" testId={V.target} workloads={workloads} value={target} onChange={setTarget} kinds={["Deployment", "StatefulSet", "DaemonSet", "ReplicaSet"]} />
                    <div className="grid gap-3 sm:grid-cols-2">
                        <div className="space-y-1.5">
                            <Label htmlFor="vpa-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="vpa-name" data-testid={V.name} value={name} onChange={(e) => setName(e.target.value)} placeholder={body.name || "my-vpa"} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="vpa-mode" className="text-xs text-muted-foreground">{t("dialogs:autoscalingCreate.vpa.updateMode")}</Label>
                            <select id="vpa-mode" data-testid={V.mode} className="select w-full" value={mode} onChange={(e) => setMode(e.target.value)}>
                                <option value="Off">{t("dialogs:autoscalingCreate.vpa.modes.off")}</option>
                                <option value="Initial">{t("dialogs:autoscalingCreate.vpa.modes.initial")}</option>
                                <option value="Recreate">{t("dialogs:autoscalingCreate.vpa.modes.recreate")}</option>
                                <option value="Auto">Auto</option>
                            </select>
                        </div>
                    </div>
                    <div className="flex items-center gap-5 text-sm">
                        <label className="flex items-center gap-2"><input type="checkbox" data-testid={V.cpu} checked={cpu} onChange={(e) => setCpu(e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" /> {t("common:fields.cpu")}</label>
                        <label className="flex items-center gap-2"><input type="checkbox" data-testid={V.memory} checked={mem} onChange={(e) => setMem(e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" /> {t("common:fields.memory")}</label>
                    </div>
                    {error ? <div className="notice danger" role="alert" data-testid={V.error}>{error}</div> : null}
                    <Preview preview={preview} yamlId={V.yaml} warnId={V.warnings} stale={!current} />
                </div>
                <DialogFooter className="gap-2">
                    <button type="button" className="btn btn-line" data-testid={V.preview} onClick={doPreview} disabled={!!busy || !target || body.controlledResources.length === 0}>
                        <Eye size={14} /> {busy === "preview" ? t("dialogs:gitops.create.rendering") : t("dialogs:gitops.create.previewYaml")}
                    </button>
                    <button type="button" className="btn btn-fill" data-testid={V.create} onClick={create} disabled={!!busy || !current}>
                        <Plus size={14} /> {busy === "create" ? t("dialogs:autoscaling.create.creating") : t("common:actions.create")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

// ---------------------------------------------------------------- KEDA
// Mirrors the server's closed allow-list: only these trigger types and settings are accepted.
const TRIGGERS = {
    cpu: { labelKey: "common:fields.cpu", fields: [{ k: "type", lKey: "dialogs:autoscalingCreate.keda.fields.metricType", options: ["Utilization", "AverageValue"] }, { k: "value", lKey: "dialogs:autoscalingCreate.keda.fields.targetValue", ph: "60" }] },
    memory: { labelKey: "common:fields.memory", fields: [{ k: "type", lKey: "dialogs:autoscalingCreate.keda.fields.metricType", options: ["Utilization", "AverageValue"] }, { k: "value", lKey: "dialogs:autoscalingCreate.keda.fields.targetValue", ph: "70" }] },
    cron: { labelKey: "dialogs:autoscalingCreate.keda.triggers.cron", fields: [{ k: "timezone", lKey: "dialogs:autoscalingCreate.keda.fields.timezone", ph: "Europe/Berlin" }, { k: "start", lKey: "dialogs:autoscalingCreate.keda.fields.start", ph: "0 8 * * 1-5" }, { k: "end", lKey: "dialogs:autoscalingCreate.keda.fields.end", ph: "0 18 * * 1-5" }, { k: "desiredReplicas", lKey: "dialogs:autoscalingCreate.keda.fields.replicasInWindow", ph: "5" }] },
    prometheus: { labelKey: "dialogs:autoscalingCreate.keda.triggers.prometheus", auth: true, fields: [{ k: "serverAddress", lKey: "dialogs:autoscalingCreate.keda.fields.serverAddress", ph: "http://prometheus.monitoring:9090" }, { k: "query", lKey: "dialogs:autoscalingCreate.keda.fields.query", ph: "sum(rate(http_requests_total[2m]))" }, { k: "threshold", lKey: "dialogs:autoscalingCreate.keda.fields.threshold", ph: "100" }] },
    kafka: { labelKey: "dialogs:autoscalingCreate.keda.triggers.kafka", auth: true, fields: [{ k: "bootstrapServers", lKey: "dialogs:autoscalingCreate.keda.fields.bootstrapServers", ph: "kafka:9092" }, { k: "consumerGroup", lKey: "dialogs:autoscalingCreate.keda.fields.consumerGroup", ph: "orders" }, { k: "topic", lKey: "dialogs:autoscalingCreate.keda.fields.topic", ph: "orders" }, { k: "lagThreshold", lKey: "dialogs:autoscalingCreate.keda.fields.lagThreshold", ph: "50" }] },
    redis: { labelKey: "dialogs:autoscalingCreate.keda.triggers.redis", auth: true, fields: [{ k: "address", lKey: "dialogs:autoscalingCreate.keda.fields.address", ph: "redis:6379" }, { k: "listName", lKey: "dialogs:autoscalingCreate.keda.fields.list", ph: "jobs" }, { k: "listLength", lKey: "dialogs:autoscalingCreate.keda.fields.lengthThreshold", ph: "10" }] },
    rabbitmq: { labelKey: "dialogs:autoscalingCreate.keda.triggers.rabbitmq", auth: true, fields: [{ k: "queueName", lKey: "dialogs:autoscalingCreate.keda.fields.queue", ph: "tasks" }, { k: "mode", lKey: "dialogs:autoscalingCreate.keda.fields.mode", options: ["QueueLength", "MessageRate"] }, { k: "value", lKey: "common:fields.value", ph: "20" }] },
    "aws-sqs-queue": { labelKey: "dialogs:autoscalingCreate.keda.triggers.awsSqs", auth: true, fields: [{ k: "queueURL", lKey: "dialogs:autoscalingCreate.keda.fields.queueUrl", ph: "https://sqs.eu-west-1.amazonaws.com/123/queue" }, { k: "queueLength", lKey: "dialogs:autoscalingCreate.keda.fields.queueLength", ph: "5" }, { k: "awsRegion", lKey: "dialogs:autoscalingCreate.keda.fields.region", ph: "eu-west-1" }] },
};

const newTrigger = (type = "cron") => ({ type, settings: Object.fromEntries(TRIGGERS[type].fields.filter((f) => f.options).map((f) => [f.k, f.options[0]])), authenticationRef: "" });

export function KedaCreateDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const workloads = useWorkloads(open);
    const [target, setTarget] = useState("");
    const [name, setName] = useState("");
    const [minR, setMinR] = useState("1");
    const [maxR, setMaxR] = useState("10");
    const [triggers, setTriggers] = useState([newTrigger()]);
    const [auths, setAuths] = useState([]);
    const [preview, setPreview] = useState(null);
    const [previewFor, setPreviewFor] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState("");

    useEffect(() => {
        if (open) {
            setTarget("");
            setName("");
            setMinR("1");
            setMaxR("10");
            setTriggers([newTrigger()]);
            setPreview(null);
            setError("");
        }
    }, [open]);

    const [ns, targetKind, targetName] = target.split("|");
    useEffect(() => {
        if (!open || !ns) {
            setAuths([]);
            return;
        }
        api.get("/triggerauthentications", { params: { namespace: ns } }).then((r) => setAuths(r.data?.items || [])).catch(() => setAuths([]));
    }, [open, ns]);

    const body = useMemo(
        () => ({
            namespace: ns,
            name: name || (targetName ? `${targetName}-scaler` : ""),
            targetKind,
            targetName,
            minReplicas: Number(minR),
            maxReplicas: Number(maxR),
            triggers: triggers.map((t) => {
                const settings = Object.fromEntries(Object.entries(t.settings).filter(([, v]) => v !== "" && v != null));
                const out = { type: t.type, settings };
                if (t.authenticationRef) out.authenticationRef = t.authenticationRef;
                return out;
            }),
        }),
        [ns, name, targetKind, targetName, minR, maxR, triggers]
    );
    const key = JSON.stringify(body);
    const current = preview && previewFor === key;
    const setTrigger = (i, patch) => setTriggers((ts) => ts.map((t, j) => (j === i ? { ...t, ...patch } : t)));

    const doPreview = async () => {
        setBusy("preview");
        setError("");
        try {
            const { data } = await api.post("/scaledobjects/preview", body);
            setPreview(data);
            setPreviewFor(key);
        } catch (e) {
            setPreview(null);
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };
    const create = async () => {
        setBusy("create");
        setError("");
        try {
            await api.post("/scaledobjects", body);
            toast.success(t("toasts:autoscalingCreate.created", { name: body.name }));
            onCreated();
            onOpenChange(false);
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-2xl" data-testid={K.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:autoscalingCreate.keda.title")}</DialogTitle>
                    <DialogDescription>{t("dialogs:autoscalingCreate.keda.description")}</DialogDescription>
                </DialogHeader>
                <div tabIndex={0} role="region" aria-label={t("dialogs:autoscalingCreate.keda.region")} className="max-h-[58vh] space-y-3 overflow-y-auto pr-1">
                    <TargetSelect id="keda-target" testId={K.target} workloads={workloads} value={target} onChange={setTarget} kinds={["Deployment", "StatefulSet"]} />
                    <div className="grid gap-3 sm:grid-cols-3">
                        <div className="space-y-1.5 sm:col-span-1">
                            <Label htmlFor="keda-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="keda-name" data-testid={K.name} value={name} onChange={(e) => setName(e.target.value)} placeholder={body.name || "my-scaler"} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="keda-min" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.minReplicas")}</Label>
                            <Input id="keda-min" data-testid={K.min} type="number" min={0} value={minR} onChange={(e) => setMinR(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="keda-max" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.maxReplicas")}</Label>
                            <Input id="keda-max" data-testid={K.max} type="number" min={1} value={maxR} onChange={(e) => setMaxR(e.target.value)} className="bg-muted/30" />
                        </div>
                    </div>

                    {triggers.map((trg, i) => {
                        const def = TRIGGERS[trg.type];
                        return (
                            <fieldset key={i} className="space-y-2 rounded-xl border border-border p-3">
                                <legend className="px-1 text-xs text-muted-foreground">{t("dialogs:autoscalingCreate.keda.trigger", { n: i + 1 })}</legend>
                                <div className="flex items-end gap-2">
                                    <div className="flex-1 space-y-1.5">
                                        <Label htmlFor={`keda-type-${i}`} className="text-xs text-muted-foreground">{t("common:fields.type")}</Label>
                                        <select id={`keda-type-${i}`} data-testid={K.triggerType(i)} className="select w-full" value={trg.type} onChange={(e) => setTrigger(i, newTrigger(e.target.value))}>
                                            {Object.entries(TRIGGERS).map(([k, v]) => (
                                                <option key={k} value={k}>{t(v.labelKey)}</option>
                                            ))}
                                        </select>
                                    </div>
                                    {triggers.length > 1 && (
                                        <button type="button" className="btn btn-danger btn-sm" data-testid={K.removeTrigger(i)} aria-label={t("dialogs:autoscalingCreate.keda.removeTrigger", { n: i + 1 })} onClick={() => setTriggers((ts) => ts.filter((_, j) => j !== i))}>
                                            <Trash2 size={13} />
                                        </button>
                                    )}
                                </div>
                                <div className="grid gap-2 sm:grid-cols-2">
                                    {def.fields.map((f) => (
                                        <div key={f.k} className="space-y-1">
                                            <Label htmlFor={`keda-${i}-${f.k}`} className="text-xs text-muted-foreground">{t(f.lKey)}</Label>
                                            {f.options ? (
                                                <select id={`keda-${i}-${f.k}`} data-testid={K.triggerField(i, f.k)} className="select w-full" value={trg.settings[f.k] || f.options[0]} onChange={(e) => setTrigger(i, { settings: { ...trg.settings, [f.k]: e.target.value } })}>
                                                    {f.options.map((o) => (
                                                        <option key={o} value={o}>{o}</option>
                                                    ))}
                                                </select>
                                            ) : (
                                                <Input id={`keda-${i}-${f.k}`} data-testid={K.triggerField(i, f.k)} value={trg.settings[f.k] || ""} onChange={(e) => setTrigger(i, { settings: { ...trg.settings, [f.k]: e.target.value } })} placeholder={f.ph} className="bg-muted/30" />
                                            )}
                                        </div>
                                    ))}
                                </div>
                                {def.auth && (
                                    <div className="space-y-1">
                                        <Label htmlFor={`keda-auth-${i}`} className="text-xs text-muted-foreground">{t("dialogs:autoscalingCreate.keda.credentials")}</Label>
                                        <select id={`keda-auth-${i}`} data-testid={K.triggerAuth(i)} className="select w-full" value={trg.authenticationRef} onChange={(e) => setTrigger(i, { authenticationRef: e.target.value })}>
                                            <option value="">{t("common:states.none")}</option>
                                            {auths.map((a) => (
                                                <option key={a.name} value={a.name}>{a.name}</option>
                                            ))}
                                        </select>
                                    </div>
                                )}
                            </fieldset>
                        );
                    })}
                    <button type="button" className="btn btn-line btn-sm" data-testid={K.addTrigger} onClick={() => setTriggers((ts) => [...ts, newTrigger("cpu")])}>
                        <Plus size={13} /> {t("dialogs:autoscalingCreate.keda.addTrigger")}
                    </button>

                    {error ? <div className="notice danger" role="alert" data-testid={K.error}>{error}</div> : null}
                    <Preview preview={preview} yamlId={K.yaml} warnId={K.warnings} stale={!current} />
                </div>
                <DialogFooter className="gap-2">
                    <button type="button" className="btn btn-line" data-testid={K.preview} onClick={doPreview} disabled={!!busy || !target}>
                        <Eye size={14} /> {busy === "preview" ? t("dialogs:gitops.create.rendering") : t("dialogs:gitops.create.previewYaml")}
                    </button>
                    <button type="button" className="btn btn-fill" data-testid={K.create} onClick={create} disabled={!!busy || !current}>
                        <Plus size={14} /> {busy === "create" ? t("dialogs:autoscaling.create.creating") : t("common:actions.create")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
