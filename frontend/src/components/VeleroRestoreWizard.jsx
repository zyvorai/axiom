import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { VELERO_RESTORE } from "@/constants/testIds";

const splitList = (s) =>
    s
        .split(",")
        .map((v) => v.trim())
        .filter(Boolean);

// "a=b, c=d" -> { a: "b", c: "d" }; null when malformed.
function parseMapping(s) {
    const out = {};
    for (const part of splitList(s)) {
        const [src, dst, ...rest] = part.split("=").map((v) => v.trim());
        if (!src || !dst || rest.length) return null;
        out[src] = dst;
    }
    return out;
}

function Field({ label, hint, testId, ...props }) {
    const id = useId();
    return (
        <div className="space-y-1.5">
            <Label htmlFor={id} className="text-xs text-muted-foreground">{label}</Label>
            <Input id={id} data-testid={testId} className="bg-muted/30" {...props} />
            {hint && <p className="text-[11px] text-ink-3">{hint}</p>}
        </div>
    );
}

const EMPTY = {
    name: "",
    included: "",
    excluded: "",
    includedResources: "",
    excludedResources: "",
    mapping: "",
    restorePVs: true,
    policy: "none",
};

/**
 * Three-step Velero restore wizard: choose, server-side preview (creates
 * nothing), typed confirmation. The server re-validates and re-checks the
 * confirmation on create.
 */
export default function VeleroRestoreWizard({ open, onOpenChange, backups, onCreated }) {
    const { t } = useTranslation();
    const [step, setStep] = useState(1);
    const [backupKey, setBackupKey] = useState("");
    const [form, setForm] = useState(EMPTY);
    const [preview, setPreview] = useState(null);
    const [confirm, setConfirm] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));
    const selectId = useId();
    const policyId = useId();

    useEffect(() => {
        if (open) {
            setStep(1);
            setForm(EMPTY);
            setPreview(null);
            setConfirm("");
            setError("");
            setBackupKey(backups[0] ? `${backups[0].namespace}/${backups[0].name}` : "");
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open]);

    const [veleroNs, backupName] = backupKey.split("/");
    const mapping = parseMapping(form.mapping);
    const body = () => ({
        namespace: veleroNs,
        name: form.name,
        backupName,
        includedNamespaces: splitList(form.included),
        excludedNamespaces: splitList(form.excluded),
        includedResources: splitList(form.includedResources),
        excludedResources: splitList(form.excludedResources),
        namespaceMapping: mapping || {},
        restorePVs: form.restorePVs,
        existingResourcePolicy: form.policy,
    });
    const errMsg = (e) => e?.response?.data?.error || e.message;

    const runPreview = async () => {
        setBusy(true);
        setError("");
        try {
            const { data } = await api.post("/velero/restores/preview", body());
            setPreview(data);
            setConfirm("");
            setStep(2);
        } catch (e) {
            setError(errMsg(e));
        } finally {
            setBusy(false);
        }
    };

    const create = async () => {
        setBusy(true);
        setError("");
        try {
            await api.post("/velero/restores", { ...body(), confirm });
            toast.success(t("toasts:backups.veleroRestoreCreated", { name: form.name }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            setError(errMsg(e));
            toast.error(t("toasts:backups.veleroRestoreFailed"), { description: errMsg(e) });
        } finally {
            setBusy(false);
        }
    };

    const tt = (k, o) => t(`dialogs:backups.veleroRestore.${k}`, o);
    const canPreview = backupKey && form.name && mapping !== null;
    const confirmed = preview && confirm === preview.confirmPhrase;

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg" data-testid={VELERO_RESTORE.dialog}>
                <DialogHeader>
                    <DialogTitle>{tt("title")}</DialogTitle>
                    <p className="text-xs text-ink-3" data-testid={VELERO_RESTORE.step}>
                        {tt("step", { n: step, total: 2 })} · {step === 1 ? tt("stepChoose") : tt("stepPreview")}
                    </p>
                </DialogHeader>

                {step === 1 && (
                    <div className="space-y-3">
                        <div className="space-y-1.5">
                            <Label htmlFor={selectId} className="text-xs text-muted-foreground">{tt("backup")}</Label>
                            <select
                                id={selectId}
                                data-testid={VELERO_RESTORE.backup}
                                className="h-9 w-full rounded-md border border-input bg-muted/30 px-2 text-sm"
                                value={backupKey}
                                onChange={(e) => setBackupKey(e.target.value)}
                            >
                                {backups.length === 0 && <option value="">{tt("noBackups")}</option>}
                                {backups.map((b) => (
                                    <option key={`${b.namespace}/${b.name}`} value={`${b.namespace}/${b.name}`}>
                                        {b.name} ({b.phase})
                                    </option>
                                ))}
                            </select>
                        </div>
                        <Field label={tt("name")} testId={VELERO_RESTORE.name} value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="restore-1" />
                        <Field label={tt("included")} hint={tt("includedHint")} testId={VELERO_RESTORE.included} value={form.included} onChange={(e) => set({ included: e.target.value })} placeholder="forge" />
                        <Field label={tt("excluded")} testId={VELERO_RESTORE.excluded} value={form.excluded} onChange={(e) => set({ excluded: e.target.value })} />
                        <Field label={tt("includedResources")} testId={VELERO_RESTORE.includedResources} value={form.includedResources} onChange={(e) => set({ includedResources: e.target.value })} placeholder="deployments,services" />
                        <Field label={tt("excludedResources")} testId={VELERO_RESTORE.excludedResources} value={form.excludedResources} onChange={(e) => set({ excludedResources: e.target.value })} />
                        <Field
                            label={tt("mapping")}
                            hint={mapping === null ? tt("mappingInvalid") : tt("mappingHint")}
                            testId={VELERO_RESTORE.mapping}
                            value={form.mapping}
                            onChange={(e) => set({ mapping: e.target.value })}
                            placeholder="forge=forge-copy"
                            aria-invalid={mapping === null}
                        />
                        <div className="space-y-1.5">
                            <Label htmlFor={policyId} className="text-xs text-muted-foreground">{tt("policy")}</Label>
                            <select
                                id={policyId}
                                data-testid={VELERO_RESTORE.policy}
                                className="h-9 w-full rounded-md border border-input bg-muted/30 px-2 text-sm"
                                value={form.policy}
                                onChange={(e) => set({ policy: e.target.value })}
                            >
                                <option value="none">{tt("policyNone")}</option>
                                <option value="update">{tt("policyUpdate")}</option>
                            </select>
                        </div>
                        <label className="flex items-center gap-2 text-sm">
                            <input type="checkbox" data-testid={VELERO_RESTORE.restorePVs} checked={form.restorePVs} onChange={(e) => set({ restorePVs: e.target.checked })} />
                            {tt("restorePVs")}
                        </label>
                    </div>
                )}

                {step === 2 && preview && (
                    <div className="space-y-3 text-sm" data-testid={VELERO_RESTORE.preview}>
                        <p>
                            <span className="mono">{preview.backup.name}</span> · {preview.backup.phase}
                            {preview.backup.totalItems > 0 && ` · ${tt("items", { count: preview.backup.itemsBackedUp, total: preview.backup.totalItems })}`}
                        </p>
                        <div>
                            <div className="text-xs text-muted-foreground">{tt("targets")}</div>
                            {preview.allNamespaces ? (
                                <p>{tt("allNamespaces")}</p>
                            ) : (
                                <ul data-testid={VELERO_RESTORE.targets}>
                                    {preview.targets.map((x) => (
                                        <li key={x.source} className="mono">
                                            {x.source} → {x.target}{" "}
                                            <span className={x.exists ? "text-tone-warn" : "text-ink-3"}>
                                                {x.exists ? tt("collides") : tt("isNew")}
                                            </span>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </div>
                        <p className="text-ink-3">{preview.resourceEffect}</p>
                        <div>
                            <div className="text-xs text-muted-foreground">{tt("warnings")}</div>
                            {preview.warnings.length === 0 ? (
                                <p>{tt("noWarnings")}</p>
                            ) : (
                                <ul className="list-disc pl-5" data-testid={VELERO_RESTORE.warnings}>
                                    {preview.warnings.map((w) => (
                                        <li key={w}>{w}</li>
                                    ))}
                                </ul>
                            )}
                        </div>
                        <Field
                            label={tt("confirmLabel", { name: preview.confirmPhrase })}
                            testId={VELERO_RESTORE.confirm}
                            value={confirm}
                            onChange={(e) => setConfirm(e.target.value)}
                            autoComplete="off"
                        />
                    </div>
                )}

                {error && (
                    <p role="alert" className="text-sm text-tone-bad" data-testid={VELERO_RESTORE.error}>
                        {error}
                    </p>
                )}

                <DialogFooter>
                    {step === 2 && (
                        <Button variant="outline" onClick={() => setStep(1)} disabled={busy} className="rounded-xl" data-testid={VELERO_RESTORE.back}>
                            {tt("back")}
                        </Button>
                    )}
                    {step === 1 ? (
                        <Button onClick={runPreview} disabled={busy || !canPreview} className="rounded-xl" data-testid={VELERO_RESTORE.previewBtn}>
                            {busy ? tt("previewing") : tt("preview")}
                        </Button>
                    ) : (
                        <Button onClick={create} disabled={busy || !confirmed} className="rounded-xl" data-testid={VELERO_RESTORE.create}>
                            {busy ? t("dialogs:backups.creating") : tt("create")}
                        </Button>
                    )}
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
