import { useCallback, useEffect, useRef, useState } from "react";
import { AlertTriangle, CheckCircle2, LogOut } from "lucide-react";
import { toast } from "sonner";
import { useTranslation, Trans } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { NODE_DRAIN as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("dialogs:nodeDrain.failed");
const gib = (b) => `${(b / 2 ** 30).toFixed(1)} GiB`;
const cpu = (m) => `${(m / 1000).toFixed(2)}`;
const VERDICT_TONE = { ok: "text-tone-green", caution: "text-tone-amber", blocked: "text-tone-red" };

function Section({ id, title, children }) {
    return (
        <section data-testid={id} className="space-y-1">
            <h4 className="text-xs font-medium text-foreground">{title}</h4>
            {children}
        </section>
    );
}

function RefList({ items, extra }) {
    return (
        <ul className="mono text-[11.5px] text-ink-3">
            {items.map((p) => (
                <li key={`${p.namespace}/${p.name}`}>
                    {p.namespace}/{p.name}
                    {extra ? ` (${extra(p)})` : ""}
                </li>
            ))}
        </ul>
    );
}

/**
 * Node maintenance wizard: Preview (server computes what a drain would do,
 * read-only) -> typed confirmation (the server enforces it too) -> cordon,
 * then drain with live progress -> uncordon.
 */
export default function NodeDrainWizard({ node, open, onClose, onChanged }) {
    const { t } = useTranslation();
    const name = node?.name;
    const [step, setStep] = useState("preview");
    const [preview, setPreview] = useState(null);
    const [error, setError] = useState("");
    const [typed, setTyped] = useState("");
    const [phase, setPhase] = useState(""); // cordon | drain | done
    const [remaining, setRemaining] = useState(null);
    const alive = useRef(true);

    const loadPreview = useCallback(async () => {
        const { data } = await api.get(`/nodes/${encodeURIComponent(name)}/drain-preview`);
        return data;
    }, [name]);

    useEffect(() => {
        alive.current = true;
        setStep("preview");
        setPreview(null);
        setError("");
        setTyped("");
        setPhase("");
        setRemaining(null);
        if (!open || !name) return undefined;
        loadPreview()
            .then((d) => alive.current && setPreview(d))
            .catch((e) => alive.current && setError(errText(e)));
        return () => {
            alive.current = false;
        };
    }, [open, name, loadPreview]);

    const run = async () => {
        setStep("run");
        setError("");
        let timer;
        try {
            if (!node.unschedulable && node.status !== "SchedulingDisabled") {
                setPhase("cordon");
                await api.post(`/nodes/${encodeURIComponent(name)}/cordon`);
                onChanged?.();
            }
            setPhase("drain");
            setRemaining(preview?.evictCount ?? null);
            timer = setInterval(() => {
                loadPreview()
                    .then((d) => alive.current && setRemaining(d.evictCount))
                    .catch(() => {});
            }, 2000);
            await api.post(`/nodes/${encodeURIComponent(name)}/drain`, null, { params: { confirm: typed } });
            setRemaining(0);
            setPhase("done");
            toast.success(t("toasts:nodes.drainRequested", { name }));
        } catch (e) {
            setError(errText(e));
            toast.error(t("toasts:nodes.drainFailed"), { description: errText(e) });
        } finally {
            clearInterval(timer);
            onChanged?.();
        }
    };

    const uncordon = async () => {
        try {
            await api.post(`/nodes/${encodeURIComponent(name)}/uncordon`);
            toast.success(t("toasts:nodes.uncordonRequested", { name }));
            onChanged?.();
            onClose();
        } catch (e) {
            setError(errText(e));
        }
    };

    if (!open || !node) return null;
    const rs = preview?.reschedule;
    const confirmed = typed === name;
    const steps = ["preview", "confirm", "run"];
    return (
        <Dialog open={open} onOpenChange={(o) => !o && phase !== "drain" && onClose()}>
            <DialogContent className="max-w-2xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:nodeDrain.title", { name })}</DialogTitle>
                    <DialogDescription data-testid={T.step} data-step={step}>
                        {t(`dialogs:nodeDrain.step.${step}`)} · {steps.indexOf(step) + 1}/{steps.length}
                    </DialogDescription>
                </DialogHeader>

                {error ? <div className="notice danger" role="alert" data-testid={T.error}>{error}</div> : null}
                {step === "preview" && !preview && !error ? <Skeleton className="h-32 rounded-xl bg-muted" /> : null}

                {step === "preview" && preview && (
                    <div tabIndex={0} role="region" aria-label={t("dialogs:nodeDrain.step.preview")} className="max-h-[60vh] space-y-3 overflow-auto text-sm">
                        <div className="flex items-start gap-2 rounded-xl border border-border p-3" data-testid={T.verdict} data-verdict={preview.verdict}>
                            {preview.verdict === "ok" ? (
                                <CheckCircle2 size={16} className={`mt-0.5 shrink-0 ${VERDICT_TONE.ok}`} aria-hidden="true" />
                            ) : (
                                <AlertTriangle size={16} className={`mt-0.5 shrink-0 ${VERDICT_TONE[preview.verdict]}`} aria-hidden="true" />
                            )}
                            <div>
                                <p className={VERDICT_TONE[preview.verdict]}>{t(`dialogs:nodeDrain.verdict.${preview.verdict}`)}</p>
                                <ul className="text-xs text-ink-3">
                                    {preview.reasons.map((r) => (
                                        <li key={r}>{t(`dialogs:nodeDrain.reason.${r}`)}</li>
                                    ))}
                                </ul>
                            </div>
                        </div>

                        {preview.pdbBlocked.length > 0 && (
                            <Section id={T.pdbBlocked} title={t("dialogs:nodeDrain.pdbBlocked")}>
                                <ul className="mono text-[11.5px] text-ink-3">
                                    {preview.pdbBlocked.map((b) => (
                                        <li key={`${b.namespace}/${b.pdb}`}>
                                            {b.namespace}/{b.pdb} ({t("dialogs:nodeDrain.pdbAllows", { n: b.disruptionsAllowed })}): {b.pods.join(", ")}
                                        </li>
                                    ))}
                                </ul>
                            </Section>
                        )}
                        <Section id={T.evict} title={t("dialogs:nodeDrain.evict", { n: preview.evictCount })}>
                            <ul className="mono text-[11.5px] text-ink-3">
                                {preview.evict.map((g) => (
                                    <li key={`${g.namespace}/${g.owner}`}>
                                        {g.namespace} · {g.owner} · {g.pods.length}
                                    </li>
                                ))}
                            </ul>
                        </Section>
                        {preview.unmanaged.length > 0 && (
                            <Section id={T.unmanaged} title={t("dialogs:nodeDrain.unmanaged")}>
                                <RefList items={preview.unmanaged} />
                            </Section>
                        )}
                        {preview.localStorage.length > 0 && (
                            <Section id={T.localStorage} title={t("dialogs:nodeDrain.localStorage")}>
                                <RefList items={preview.localStorage} />
                            </Section>
                        )}
                        {preview.singleReplica.length > 0 && (
                            <Section id={T.singleReplica} title={t("dialogs:nodeDrain.singleReplica")}>
                                <RefList items={preview.singleReplica} extra={(p) => p.owner} />
                            </Section>
                        )}
                        {preview.pinnedVolumes.length > 0 && (
                            <Section id={T.pinned} title={t("dialogs:nodeDrain.pinned")}>
                                <RefList items={preview.pinnedVolumes} extra={(p) => p.reason} />
                            </Section>
                        )}
                        {preview.ignored.length > 0 && (
                            <Section id={T.ignored} title={t("dialogs:nodeDrain.ignored", { n: preview.ignored.length })}>
                                <RefList items={preview.ignored} extra={(p) => t(`dialogs:nodeDrain.ignoredReason.${p.reason}`)} />
                            </Section>
                        )}
                        <Section id={T.reschedule} title={t("dialogs:nodeDrain.reschedule")}>
                            <p className={rs.fits ? "text-xs" : "text-xs text-tone-amber"}>
                                {rs.fits ? t("dialogs:nodeDrain.fits") : t("dialogs:nodeDrain.noFit")}{" "}
                                {t("dialogs:nodeDrain.rescheduleBody", {
                                    nodes: rs.candidateNodes,
                                    cpu: cpu(rs.headroomCpuMilli),
                                    mem: gib(rs.headroomMemBytes),
                                    needCpu: cpu(rs.needCpuMilli),
                                    needMem: gib(rs.needMemBytes),
                                })}
                            </p>
                            <p className="text-[11px] text-ink-3">{t("dialogs:nodeDrain.approximate")}</p>
                        </Section>
                    </div>
                )}

                {step === "confirm" && (
                    <div className="space-y-2">
                        <p className="text-sm">{t("dialogs:nodeDrain.confirmHelp")}</p>
                        <Label htmlFor="node-drain-confirm" className="text-xs text-muted-foreground">
                            <Trans i18nKey="dialogs:nodeDrain.confirmLabel" values={{ name }} components={{ code: <span className="mono" /> }} />
                        </Label>
                        <Input id="node-drain-confirm" data-testid={T.confirmInput} value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" className="bg-muted/30" />
                    </div>
                )}

                {step === "run" && (
                    <div className="space-y-2 text-sm" data-testid={T.progress} data-phase={phase}>
                        <p>{phase === "cordon" ? t("dialogs:nodeDrain.progressCordon") : null}</p>
                        <p>{phase === "drain" ? t("dialogs:nodeDrain.progressDrain", { n: remaining ?? "?" }) : null}</p>
                        {phase === "done" ? (
                            <p className="flex items-center gap-2 text-tone-green" data-testid={T.done}>
                                <CheckCircle2 size={16} aria-hidden="true" /> {t("dialogs:nodeDrain.done", { name })}
                            </p>
                        ) : null}
                    </div>
                )}

                <DialogFooter>
                    {step === "confirm" && (
                        <button type="button" className="btn btn-line" data-testid={T.back} onClick={() => setStep("preview")}>
                            {t("common:actions.back")}
                        </button>
                    )}
                    {step === "preview" && (
                        <button type="button" className="btn btn-primary" data-testid={T.next} disabled={!preview} onClick={() => setStep("confirm")}>
                            {t("common:actions.continue")}
                        </button>
                    )}
                    {step === "confirm" && (
                        <button type="button" className="btn btn-danger" data-testid={T.start} disabled={!confirmed} onClick={run}>
                            <LogOut size={14} /> {t("dialogs:nodeDrain.start")}
                        </button>
                    )}
                    {step === "run" && (phase === "done" || error) && (
                        <>
                            <button type="button" className="btn btn-line" data-testid={T.uncordon} onClick={uncordon}>
                                <CheckCircle2 size={14} /> {t("pages:nodes.actions.uncordon")}
                            </button>
                            <button type="button" className="btn btn-primary" onClick={onClose}>
                                {t("common:actions.close")}
                            </button>
                        </>
                    )}
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
