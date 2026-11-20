import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import ApplyManifestDialog from "@/components/ApplyManifestDialog";
import MacTerm from "@/components/MacTerm";
import StatusBadge from "@/components/StatusBadge";
import SegmentedProgress from "@/components/SegmentedProgress";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import Stepper from "@/components/Stepper";
import {
    DropdownMenu,
    DropdownMenuTrigger,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import {
    MoreHorizontal,
    Play,
    Square,
    RotateCw,
    Lock,
    Unlock,
    Trash2,
    Rocket,
    GitCommitHorizontal,
    Plus,
    Boxes,
} from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/components/ConfirmDialog";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";

// Display labels for the rollout actions; the action id itself is sent to the API.
const ROLLOUT_ACTION_KEYS = {
    restart: "dialogs:workloads.rollout.actions.restart",
    pause: "dialogs:workloads.rollout.actions.pause",
    resume: "dialogs:workloads.rollout.actions.resume",
    undo: "dialogs:workloads.rollout.actions.undo",
};

function RolloutDialog({ workload, onOpenChange }) {
    const { t } = useTranslation();
    const [status, setStatus] = useState(null);
    const [busy, setBusy] = useState("");

    const load = () =>
        api
            .get(`/workloads/${workload.namespace}/${workload.kind}/${encodeURIComponent(workload.name)}/rollout`)
            .then((r) => setStatus(r.data))
            .catch(() => setStatus(false));

    useEffect(() => {
        load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [workload]);

    const act = async (action) => {
        setBusy(action);
        try {
            await api.post(`/workloads/${workload.namespace}/${workload.kind}/${encodeURIComponent(workload.name)}/rollout/${action}`);
            toast.success(t("toasts:workloads.actionRequested", { action }));
            load();
        } catch (e) {
            toast.error(t("toasts:workloads.actionFailed", { action }), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">{t("dialogs:workloads.rollout.title", { name: workload.name })}</DialogTitle>
                </DialogHeader>
                {status === null && <Skeleton className="h-40 rounded-xl bg-muted" />}
                {status === false && <div className="py-8 text-center text-sm text-muted-foreground">{t("dialogs:workloads.rollout.notSupported", { kind: workload.kind })}</div>}
                {status && (
                    <div className="space-y-4">
                        <div className="grid grid-cols-4 gap-3 text-center text-xs">
                            {[
                                ["status", t("common:fields.status"), status.status],
                                ["replicas", t("common:fields.replicas"), status.replicas],
                                ["ready", t("common:fields.ready"), status.ready],
                                ["available", t("dialogs:workloads.rollout.available"), status.available],
                            ].map(([id, label, value]) => (
                                <div key={id} className="border-y border-border p-2.5">
                                    <div className="text-[11px] uppercase tracking-widest text-muted-foreground">{label}</div>
                                    <div className="mono mt-1 truncate text-sm">{value}</div>
                                </div>
                            ))}
                        </div>
                        <SegmentedProgress
                            segments={[
                                { key: "available", label: t("dialogs:workloads.rollout.available"), value: status.available, tone: "done" },
                                { key: "updating", label: t("dialogs:workloads.rollout.updating"), value: Math.max(0, status.ready - status.available), tone: "active" },
                                { key: "pending", label: t("dialogs:workloads.rollout.pending"), value: Math.max(0, status.replicas - status.ready), tone: "pending" },
                            ]}
                        />
                        {(status.history || []).length > 0 && (
                            <div className="rounded-xl border">
                                {status.history.map((h) => (
                                    <div key={h.revision} className="flex items-center justify-between border-b px-3 py-2 text-xs last:border-none">
                                        <span className="mono">{t("dialogs:workloads.rollout.revision", { revision: h.revision })}</span>
                                        <span className="text-muted-foreground">{h.status}</span>
                                        <span className="mono text-muted-foreground">{h.age}</span>
                                    </div>
                                ))}
                            </div>
                        )}
                        <div className="flex flex-wrap gap-2">
                            {["restart", "pause", "resume", "undo"].map((a) => (
                                <Button
                                    key={a}
                                    size="sm"
                                    variant="outline"
                                    disabled={!!busy}
                                    onClick={() => act(a)}
                                    className="rounded-xl bg-muted/30 capitalize"
                                >
                                    {t(ROLLOUT_ACTION_KEYS[a])}
                                </Button>
                            ))}
                        </div>
                    </div>
                )}
            </DialogContent>
        </Dialog>
    );
}

// A Kubernetes object name (RFC 1123 label) — what the API server would enforce anyway.
const DNS_LABEL = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/;

function ScaleDialog({ workload, onOpenChange, onDone }) {
    const { t } = useTranslation();
    const [replicas, setReplicas] = useState(String(workload.desired ?? workload.ready ?? 1));
    const [busy, setBusy] = useState(false);
    const count = replicas.trim() === "" ? NaN : Number(replicas); // Number("") is 0, which would silently stop the workload
    const valid = Number.isInteger(count) && count >= 0;

    const submit = async () => {
        if (!valid) return;
        setBusy(true);
        try {
            await api.post(`/workloads/${workload.namespace}/${workload.kind}/${encodeURIComponent(workload.name)}/scale`, {
                replicas: count,
            });
            toast.success(t("toasts:workloads.scaled", { name: workload.name, replicas }));
            onOpenChange(false);
            onDone();
        } catch (e) {
            toast.error(t("toasts:workloads.scaleFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="max-w-xs">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">{t("dialogs:workloads.scale.title", { name: workload.name })}</DialogTitle>
                </DialogHeader>
                <Input type="number" min={0} step={1} aria-label={t("common:fields.replicas")} aria-invalid={!valid} value={replicas} onChange={(e) => setReplicas(e.target.value)} className="bg-muted/30" />
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !valid} className="rounded-xl">
                        {busy ? t("dialogs:workloads.scale.scaling") : t("dialogs:workloads.scale.submit")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function WorkloadActions({ w, onChanged, onScale, onRollout }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [busy, setBusy] = useState("");
    // A DaemonSet has no replica count in Kubernetes' own model — one Pod
    // per matching node, not a scalable target — so Start/Stop/Scale/
    // Rollout/Seal/Unseal all correctly 400 server-side. Restart and
    // Delete do apply and stay available.
    const isDaemonSet = w.kind === "DaemonSet";

    const act = async (action) => {
        setBusy(action);
        try {
            await api.post(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}/${action}`);
            toast.success(t("toasts:workloads.workloadActionRequested", { name: w.name, action }));
            onChanged();
        } catch (e) {
            toast.error(t("toasts:workloads.actionFailed", { action }), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const stillAct = async (action) => {
        setBusy(action);
        try {
            const { data: preview } = await api.get(
                `/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}/still`,
                { params: { action } },
            );
            const titleKey =
                action === "sleep"
                    ? "pages:workloads.still.sleepTitle"
                    : action === "end"
                      ? "pages:workloads.still.endTitle"
                      : action === "hush"
                        ? "pages:workloads.still.hushTitle"
                        : "pages:workloads.still.title";
            const descKey =
                action === "sleep"
                    ? "pages:workloads.still.sleepDescription"
                    : action === "end"
                      ? "pages:workloads.still.endDescription"
                      : "pages:workloads.still.description";
            const ok = await confirm({
                title: t(titleKey, { name: w.name }),
                description: t(descKey, {
                    sessions: preview.sessions || 0,
                    windows: preview.windows || 0,
                    replicas: preview.replicas || 0,
                    atlas: (preview.atlas || []).length,
                }),
                confirmLabel: t(`pages:workloads.menu.${action}`, { defaultValue: action }),
            });
            if (!ok) return;
            await api.post(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}/still`, { action });
            toast.success(t("toasts:workloads.workloadActionRequested", { name: w.name, action }));
            onChanged();
        } catch (e) {
            toast.error(t("toasts:workloads.actionFailed", { action }), {
                description: e?.response?.data?.error || e.message,
            });
        } finally {
            setBusy("");
        }
    };

    const remove = async () => {
        const ok = await confirm({
            title: t("dialogs:workloads.delete.title", { kind: w.kind, name: w.name }),
            description: t("dialogs:workloads.delete.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        setBusy("delete");
        try {
            await api.delete(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`);
            toast.success(t("toasts:workloads.deleted", { name: w.name }));
            onChanged();
        } catch (e) {
            toast.error(t("toasts:workloads.deleteFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button size="sm" variant="ghost" disabled={!!busy} aria-label={t("pages:workloads.actionsFor", { name: w.name })} title={t("pages:workloads.actionsFor", { name: w.name })} className="h-8 w-8 p-0 text-muted-foreground">
                    <MoreHorizontal className="h-4 w-4" />
                </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-44">
                {!isDaemonSet && (
                    <>
                        <DropdownMenuItem onClick={() => act("start")}>
                            <Play className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.start")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => act("stop")}>
                            <Square className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.stop")}
                        </DropdownMenuItem>
                    </>
                )}
                <DropdownMenuItem onClick={() => act("restart")}>
                    <RotateCw className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.restart")}
                </DropdownMenuItem>
                {!isDaemonSet && (
                    <>
                        <DropdownMenuItem onClick={onScale}>
                            <GitCommitHorizontal className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.scale")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={onRollout}>
                            <Rocket className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.rollout")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={() => stillAct("hush")}>
                            <Lock className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.hush")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => stillAct("seal")}>
                            <Lock className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.seal")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => stillAct("sleep")}>
                            <Square className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.sleep")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => stillAct("end")} className="text-tone-red focus:text-tone-red">
                            <Trash2 className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.end")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => act("unseal")}>
                            <Unlock className="mr-2 h-3.5 w-3.5" /> {t("pages:workloads.menu.unseal")}
                        </DropdownMenuItem>
                    </>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={remove} className="text-tone-red focus:text-tone-red">
                    <Trash2 className="mr-2 h-3.5 w-3.5" /> {t("common:actions.delete")}
                </DropdownMenuItem>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}

// The intent parser (internal/platform/intent.go) understands English keywords
// only ("run", "copies"/"replicas", "image", "port"), so the example stays
// English in every language rather than suggesting phrasing it cannot parse.
const INTENT_EXAMPLE = "a small stateless api on 2 replicas running ghcr.io/zyvorai/checkout:v1 exposed on port 8080";

function LaunchWizard({ open, onOpenChange, onLaunched }) {
    const { t } = useTranslation();
    const [form, setForm] = useState({
        namespace: "default",
        name: "",
        image: "",
        replicas: 1,
        port: 8080,
        cpu: "250m",
        memory: "256Mi",
        expose: false,
    });
    const [intent, setIntent] = useState("");
    const [intentPreview, setIntentPreview] = useState(null);
    const [manifest, setManifest] = useState("");
    const [validation, setValidation] = useState(null);
    const [applyOpen, setApplyOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const [step, setStep] = useState("form");
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const nameOk = !form.name || DNS_LABEL.test(form.name);

    const launchForm = async () => {
        if (!nameOk) return;
        setBusy(true);
        try {
            await api.post("/workloads", form);
            toast.success(t("toasts:workloads.launched", { name: form.name }));
            onOpenChange(false);
            onLaunched();
        } catch (e) {
            toast.error(t("toasts:workloads.launchFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const parseIntent = async () => {
        setBusy(true);
        try {
            const { data } = await api.post("/intent/parse", { intent });
            setIntentPreview(data);
        } catch (e) {
            toast.error(t("toasts:workloads.parseFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const placeIntent = async () => {
        setBusy(true);
        try {
            await api.post("/intent/place", { intent });
            toast.success(t("toasts:workloads.placed"));
            onOpenChange(false);
            onLaunched();
        } catch (e) {
            toast.error(t("toasts:workloads.placeFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const validate = async () => {
        setBusy(true);
        try {
            const { data } = await api.post("/validate", { manifest });
            setValidation(data);
        } catch (e) {
            setValidation({ valid: false, errors: [e?.response?.data?.error || e.message] });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:workloads.launch.title")}</DialogTitle>
                </DialogHeader>
                <Tabs value={step} onValueChange={setStep}>
                    <Stepper
                        className="mb-1"
                        active={step}
                        onSelect={setStep}
                        steps={[
                            { key: "form", label: t("dialogs:workloads.launch.steps.form") },
                            { key: "intent", label: t("dialogs:workloads.launch.steps.intent") },
                            { key: "yaml", label: t("dialogs:workloads.launch.steps.yaml") },
                        ]}
                    />

                    <TabsContent value="form" className="mt-4 space-y-3">
                        <div className="grid grid-cols-2 gap-3">
                            <div className="space-y-1.5">
                                <Label htmlFor="workloads-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                                <Input id="workloads-namespace" value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="workloads-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                                <Input id="workloads-name" value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="checkout-worker" aria-invalid={!nameOk} aria-describedby={nameOk ? undefined : "workloads-name-err"} className="bg-muted/30" />
                                {!nameOk && (
                                    <p id="workloads-name-err" className="text-xs text-tone-red">
                                        {t("errors:workloads.dnsLabel")}
                                    </p>
                                )}
                            </div>
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="workloads-image" className="text-xs text-muted-foreground">{t("common:fields.image")}</Label>
                            <Input id="workloads-image" value={form.image} onChange={(e) => set({ image: e.target.value })} placeholder="ghcr.io/zyvorai/checkout:v1" className="bg-muted/30" />
                        </div>
                        <div className="grid grid-cols-3 gap-3">
                            <div className="space-y-1.5">
                                <Label htmlFor="workloads-replicas" className="text-xs text-muted-foreground">{t("common:fields.replicas")}</Label>
                                <Input id="workloads-replicas" type="number" min={1} value={form.replicas} onChange={(e) => set({ replicas: Number(e.target.value) })} className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="workloads-cpu" className="text-xs text-muted-foreground">{t("common:fields.cpu")}</Label>
                                <Input id="workloads-cpu" value={form.cpu} onChange={(e) => set({ cpu: e.target.value })} className="bg-muted/30" />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="workloads-memory" className="text-xs text-muted-foreground">{t("common:fields.memory")}</Label>
                                <Input id="workloads-memory" value={form.memory} onChange={(e) => set({ memory: e.target.value })} className="bg-muted/30" />
                            </div>
                        </div>
                        <div className="flex items-center justify-between border-y border-border px-3 py-2.5">
                            <div className="space-y-1.5">
                                <Label htmlFor="workloads-port" className="text-xs text-muted-foreground">{t("dialogs:workloads.launch.port")}</Label>
                                <Input id="workloads-port" type="number" value={form.port} onChange={(e) => set({ port: Number(e.target.value) })} className="h-7 w-24 bg-muted/30" />
                            </div>
                            <div className="flex items-center gap-2">
                                <Label htmlFor="workloads-expose-service" className="text-xs text-muted-foreground">{t("dialogs:workloads.launch.exposeService")}</Label>
                                <Switch id="workloads-expose-service" aria-label={t("dialogs:workloads.launch.exposeService")} checked={form.expose} onCheckedChange={(v) => set({ expose: v })} />
                            </div>
                        </div>
                        <DialogFooter>
                            <Button variant="brand" onClick={launchForm} disabled={busy || !form.name || !nameOk || !form.image} className="rounded-xl">
                                {busy ? t("dialogs:workloads.launch.launching") : t("dialogs:workloads.launch.launch")}
                            </Button>
                        </DialogFooter>
                    </TabsContent>

                    <TabsContent value="intent" className="mt-4 space-y-3">
                        <MacTerm title="intent.txt" bodyClassName="!p-0">
                            <Textarea
                                value={intent}
                                onChange={(e) => setIntent(e.target.value)}
                                placeholder={INTENT_EXAMPLE}
                                className="macos-term-body mono min-h-[100px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0"
                            />
                        </MacTerm>
                        {intentPreview && (
                            <MacTerm
                                title={t("dialogs:workloads.launch.receipt")}
                                bodyClassName="max-h-40 overflow-auto whitespace-pre-wrap px-4 py-3 text-[11px] leading-[1.55] text-term-punct"
                            >
                                {intentPreview.receipt}
                            </MacTerm>
                        )}
                        <DialogFooter className="gap-2">
                            <Button variant="outline" onClick={parseIntent} disabled={busy || !intent} className="rounded-xl bg-muted/30">
                                {t("dialogs:workloads.launch.preview")}
                            </Button>
                            <Button onClick={placeIntent} disabled={busy || !intent} className="rounded-xl">
                                {busy ? t("dialogs:workloads.launch.placing") : t("dialogs:workloads.launch.place")}
                            </Button>
                        </DialogFooter>
                    </TabsContent>

                    <TabsContent value="yaml" className="mt-4 space-y-3">
                        <MacTerm title="manifest.yaml" bodyClassName="!p-0">
                            <Textarea
                                value={manifest}
                                onChange={(e) => {
                                    setManifest(e.target.value);
                                    setValidation(null); // the last result no longer describes this text
                                }}
                                placeholder={"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: example\n..."}
                                className="macos-term-body mono min-h-[140px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0"
                                spellCheck={false}
                            />
                        </MacTerm>
                        {validation && (
                            <div className={`rounded-lg border px-3 py-2 text-xs ${validation.valid ? "border-soft-success-border bg-soft-success text-tone-green" : "border-soft-danger-border bg-soft-danger text-tone-red"}`}>
                                {validation.valid ? (validation.kind ? t("dialogs:workloads.launch.validKind", { kind: validation.kind }) : t("dialogs:workloads.launch.validManifest")) : (validation.errors || [validation.message]).join(", ")}
                            </div>
                        )}
                        <DialogFooter className="gap-2">
                            <Button variant="outline" onClick={validate} disabled={busy || !manifest} className="rounded-xl bg-muted/30">
                                {t("dialogs:workloads.launch.validate")}
                            </Button>
                            <Button
                                onClick={() => setApplyOpen(true)}
                                disabled={!validation?.valid}
                                className="rounded-xl"
                            >
                                {t("common:actions.apply")}
                            </Button>
                        </DialogFooter>
                    </TabsContent>
                </Tabs>
            </DialogContent>
            <ApplyManifestDialog
                open={applyOpen}
                onOpenChange={setApplyOpen}
                title={t("dialogs:workloads.launch.applyTitle")}
                initialManifest={manifest}
                onApplied={() => {
                    setApplyOpen(false);
                    onOpenChange(false);
                    onLaunched();
                }}
            />
        </Dialog>
    );
}

export default function Workloads() {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();
    const [kind, setKind] = useUrlState("kind", "all");
    const [scaleTarget, setScaleTarget] = useState(null);
    const [rolloutTarget, setRolloutTarget] = useState(null);
    const [launchOpen, setLaunchOpen] = useState(false);
    const [selected, setSelected] = useState(new Set());
    const [bulkBusy, setBulkBusy] = useState(false);

    const load = () => api.get("/workloads").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/workloads").then((r) => alive && setData(r.data));
        tick();
        const timer = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const { namespaces, kinds, rows } = useMemo(() => {
        if (!data) return { namespaces: [], kinds: [], rows: [] };
        const namespaces = [...new Set([...knownNamespaces, ...data.map((w) => w.namespace)])].sort();
        const kinds = [...new Set(data.map((w) => w.kind))].sort();
        const rows = data.filter(
            (w) =>
                (ns === "all" || w.namespace === ns) &&
                (kind === "all" || w.kind === kind) &&
                (q === "" ||
                    w.name.toLowerCase().includes(q.toLowerCase()) ||
                    w.image.toLowerCase().includes(q.toLowerCase()))
        );
        return { namespaces, kinds, rows };
    }, [data, q, ns, kind, knownNamespaces]);

    const key = (w) => `${w.namespace}/${w.kind}/${w.name}`;
    const toggle = (w, checked) => {
        setSelected((s) => {
            const next = new Set(s);
            if (checked) next.add(key(w));
            else next.delete(key(w));
            return next;
        });
    };
    const toggleAll = (checked) => setSelected(checked ? new Set(rows.map(key)) : new Set());

    const bulkAction = async (action) => {
        const targets = rows.filter((w) => selected.has(key(w)));
        // rows reflects the last poll, which can be stale relative to
        // `selected` — a selected row may have been deleted elsewhere (or
        // filtered out) in the up-to-10s since it was picked. Track that
        // gap explicitly rather than silently acting on fewer items than
        // the user actually selected.
        const skipped = selected.size - targets.length;
        if (action === "delete") {
            const ok = await confirm({
                title: t("dialogs:workloads.bulkDelete.title", { count: targets.length }),
                description: t("dialogs:workloads.bulkDelete.description"),
                confirmLabel: t("common:actions.delete"),
            });
            if (!ok) return;
        }
        setBulkBusy(true);
        try {
            const results = await Promise.allSettled(
                targets.map((w) =>
                    action === "delete"
                        ? api.delete(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`)
                        : api.post(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}/${action}`)
                )
            );
            const failed = results.filter((r) => r.status === "rejected").length;
            const note = skipped > 0 ? t("toasts:workloads.skippedNote", { count: skipped }) : "";
            if (failed)
                toast.error(
                    action === "delete"
                        ? t("toasts:workloads.bulkDeleteFailed", { failed, total: targets.length, note })
                        : t("toasts:workloads.bulkRestartFailed", { failed, total: targets.length, note })
                );
            else
                toast.success(
                    action === "delete"
                        ? t("toasts:workloads.bulkDeleted", { count: targets.length, note })
                        : t("toasts:workloads.bulkRestarted", { count: targets.length, note })
                );
            setSelected(new Set());
            load();
        } finally {
            setBulkBusy(false);
        }
    };

    if (!data) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title={t("pages:workloads.title")}
                description={t("pages:workloads.description")}
                action={
                    <>
                        {selected.size > 0 && (
                            <div className="flex items-center gap-2 border-y border-border px-3 py-1.5">
                                <span className="text-xs text-muted-foreground">{t("common:count.selected", { count: selected.size })}</span>
                                <Button size="sm" variant="outline" disabled={bulkBusy} onClick={() => bulkAction("restart")} className="h-7 rounded-lg bg-muted/30 text-xs">
                                    <RotateCw className="mr-1 h-3 w-3" /> {t("pages:workloads.menu.restart")}
                                </Button>
                                <Button size="sm" variant="outline" disabled={bulkBusy} onClick={() => bulkAction("delete")} className="h-7 rounded-lg border-soft-danger-border bg-soft-danger text-xs text-tone-red hover:bg-soft-danger">
                                    <Trash2 className="mr-1 h-3 w-3" /> {t("common:actions.delete")}
                                </Button>
                            </div>
                        )}
                        <button type="button" className="btn btn-fill" onClick={() => setLaunchOpen(true)}>
                            <Plus size={15} /> {t("pages:workloads.runWorkload")}
                        </button>
                    </>
                }
                tools={
                    <>
                        <SearchBox placeholder={t("pages:workloads.searchPlaceholder")} value={q} onChange={setQ} className="relative" />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <Select value={kind} onValueChange={setKind}>
                            <SelectTrigger aria-label={t("pages:workloads.filterByKind")} className="w-[160px] bg-muted/30">
                                <SelectValue placeholder={t("common:fields.kind")} />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">{t("pages:workloads.allKinds")}</SelectItem>
                                {kinds.map((k) => (
                                    <SelectItem key={k} value={k}>
                                        {k}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                        <SavedViews
                            pageKey="workloads"
                            filters={{ q, ns, kind }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                                setKind(f.kind ?? "all");
                            }}
                        />
                    </>
                }
            />

            <GlassCard className="overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:workloads.title")}>
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="hover:bg-transparent">
                                <TableHead className="w-10">
                                    <Checkbox
                                        aria-label={t("pages:workloads.selectAll")}
                                        checked={rows.length > 0 && selected.size === rows.length}
                                        onCheckedChange={toggleAll}
                                    />
                                </TableHead>
                                <TableHead>{t("common:fields.name")}</TableHead>
                                <TableHead>{t("common:fields.namespace")}</TableHead>
                                <TableHead>{t("common:fields.kind")}</TableHead>
                                <TableHead>{t("pages:workloads.columns.desired")}</TableHead>
                                <TableHead>{t("common:fields.ready")}</TableHead>
                                <TableHead>{t("pages:workloads.columns.available")}</TableHead>
                                <TableHead>{t("common:fields.status")}</TableHead>
                                <TableHead>{t("common:fields.age")}</TableHead>
                                <TableHead><span className="sr-only">{t("common:fields.actions")}</span></TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((w) => (
                                <TableRow
                                    key={`${w.namespace}/${w.name}`}
                                    className="hover:bg-muted"
                                >
                                    <TableCell>
                                        <Checkbox aria-label={t("pages:workloads.selectOne", { name: w.name })} checked={selected.has(key(w))} onCheckedChange={(v) => toggle(w, v)} />
                                    </TableCell>
                                    <TableCell className="max-w-xs">
                                        <Link
                                            to={`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`}
                                            className="block truncate font-medium hover:underline"
                                            title={w.name}
                                        >
                                            {w.name}
                                        </Link>
                                        <div className="mono truncate text-[11px] text-muted-foreground">{w.image}</div>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{w.namespace}</TableCell>
                                    <TableCell className="text-xs">{w.kind}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.desired}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.ready}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.available}</TableCell>
                                    <TableCell>
                                        <StatusBadge status={w.status} />
                                    </TableCell>
                                    <TableCell className="mono text-xs">{w.age}</TableCell>
                                    <TableCell>
                                        <WorkloadActions
                                            w={w}
                                            onChanged={load}
                                            onScale={() => setScaleTarget(w)}
                                            onRollout={() => setRolloutTarget(w)}
                                        />
                                    </TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && data.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={10} className="py-16">
                                        <div className="flex flex-col items-center gap-3 text-center">
                                            <div className="grid h-12 w-12 place-items-center rounded-2xl bg-muted">
                                                <Boxes className="h-5 w-5 text-muted-foreground" />
                                            </div>
                                            <div>
                                                <div className="text-sm font-medium">{t("pages:workloads.empty.title")}</div>
                                                <div className="mt-0.5 text-xs text-muted-foreground">
                                                    {t("pages:workloads.empty.description")}
                                                </div>
                                            </div>
                                            <Button variant="brand" size="sm" onClick={() => setLaunchOpen(true)} className="rounded-xl">
                                                <Plus className="mr-1.5 h-3.5 w-3.5" /> {t("pages:workloads.empty.action")}
                                            </Button>
                                        </div>
                                    </TableCell>
                                </TableRow>
                            )}
                            {rows.length === 0 && data.length > 0 && (
                                <TableRow>
                                    <TableCell colSpan={10} className="py-10 text-center text-sm text-muted-foreground">
                                        {t("pages:workloads.noMatches")}
                                    </TableCell>
                                </TableRow>
                            )}
                        </TableBody>
                    </Table>
                </div>
            </GlassCard>

            {scaleTarget && <ScaleDialog workload={scaleTarget} onOpenChange={(o) => !o && setScaleTarget(null)} onDone={load} />}
            {rolloutTarget && <RolloutDialog workload={rolloutTarget} onOpenChange={(o) => !o && setRolloutTarget(null)} />}
            <LaunchWizard open={launchOpen} onOpenChange={setLaunchOpen} onLaunched={load} />
        </div>
    );
}
