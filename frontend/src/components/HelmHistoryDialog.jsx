import { useEffect, useState } from "react";
import { AlertTriangle, ArrowLeft, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { useTranslation, Trans } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { HELM_HISTORY as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:generic");

function DiffBlock({ lines, testId, title }) {
    const { t } = useTranslation();
    const changed = (lines || []).some((l) => l.op !== " ");
    return (
        <div className="space-y-1" data-testid={testId}>
            <h4 className="text-xs font-medium text-muted-foreground">{title}</h4>
            {!changed ? (
                <p className="text-xs text-ink-3">{t("dialogs:helmHistory.noChanges")}</p>
            ) : (
                <pre className="mono max-h-56 overflow-auto rounded-lg border border-border p-2 text-[11.5px] leading-[1.5]" tabIndex={0}>
                    {lines
                        .filter((l) => l.op !== " ")
                        .map((l, i) => (
                            <div key={i} className={l.op === "+" ? "text-tone-green" : "text-tone-red"}>
                                {l.op} {l.text}
                            </div>
                        ))}
                </pre>
            )}
        </div>
    );
}

/**
 * History of a Helm release: revisions, a values/manifest diff between two of
 * them, and a reviewed rollback (server preview, then typed release name).
 */
export default function HelmHistoryDialog({ release, onClose, onChanged }) {
    const { t } = useTranslation();
    const { isViewer } = useAuth();
    const [revs, setRevs] = useState(null);
    const [error, setError] = useState("");
    const [from, setFrom] = useState("");
    const [to, setTo] = useState("");
    const [diff, setDiff] = useState(null);
    const [preview, setPreview] = useState(null);
    const [typed, setTyped] = useState("");
    const [busy, setBusy] = useState(false);
    const base = release ? `/helm/releases/${release.namespace}/${encodeURIComponent(release.name)}` : "";

    useEffect(() => {
        setRevs(null);
        setError("");
        setDiff(null);
        setPreview(null);
        setTyped("");
        if (!release) return undefined;
        let alive = true;
        api
            .get(`${base}/revisions`)
            .then((r) => {
                if (!alive) return;
                setRevs(r.data || []);
                const list = r.data || [];
                setTo(list[0] ? String(list[0].revision) : "");
                setFrom(list[1] ? String(list[1].revision) : "");
            })
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [release, base]);

    const compare = async () => {
        setError("");
        setBusy(true);
        try {
            const { data } = await api.get(`${base}/diff`, { params: { from, to } });
            setDiff(data);
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    const review = async (rev) => {
        setError("");
        setBusy(true);
        try {
            const { data } = await api.get(`${base}/rollback-preview`, { params: { revision: rev } });
            setPreview(data);
            setTyped("");
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    const rollback = async () => {
        setBusy(true);
        setError("");
        try {
            await api.post(`${base}/rollback`, { revision: preview.target, confirm: typed });
            toast.success(t("toasts:ops.helm.rolledBack", { name: release.name }));
            onChanged();
            onClose();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    if (!release) return null;
    return (
        <Dialog open={!!release} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-2xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:helmHistory.title", { name: release.name })}</DialogTitle>
                    <DialogDescription>{release.namespace}</DialogDescription>
                </DialogHeader>

                {!revs && !error ? <Skeleton className="h-24 rounded-xl bg-muted" /> : null}
                {error ? <div className="notice danger" role="alert" data-testid={T.error}>{error}</div> : null}

                {revs && !preview && (
                    <div className="space-y-3">
                        {revs.length === 0 ? (
                            <p className="text-sm text-ink-3">{t("dialogs:helmHistory.empty")}</p>
                        ) : (
                            <div className="max-h-56 overflow-auto" tabIndex={0} role="region" aria-label={t("dialogs:helmHistory.revisions")}>
                                <table className="tbl">
                                    <thead>
                                        <tr>
                                            <th>{t("dialogs:helmHistory.revision")}</th>
                                            <th>{t("common:fields.status")}</th>
                                            <th>{t("dialogs:helmHistory.chartVersion")}</th>
                                            <th>{t("dialogs:helmHistory.appVersion")}</th>
                                            <th>{t("dialogs:helmHistory.updated")}</th>
                                            <th>{t("dialogs:helmHistory.description")}</th>
                                            <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {revs.map((r) => (
                                            <tr key={r.revision} data-testid={T.row(r.revision)}>
                                                <td className="num">{r.revision}</td>
                                                <td>{r.status}</td>
                                                <td className="mono text-[12px]">{r.chartVersion}</td>
                                                <td className="mono text-[12px]">{r.appVersion}</td>
                                                <td className="text-[12px]">{r.updated}</td>
                                                <td className="text-[12px]">{r.description}</td>
                                                <td>
                                                    {r.status !== "deployed" && !isViewer && (
                                                        <button type="button" className="btn btn-line btn-sm" disabled={busy} data-testid={T.reviewRollback(r.revision)} onClick={() => review(r.revision)} title={t("dialogs:helmHistory.reviewRollback", { n: r.revision })} aria-label={t("dialogs:helmHistory.reviewRollback", { n: r.revision })}>
                                                            <RotateCcw size={13} />
                                                        </button>
                                                    )}
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        )}

                        {revs.length > 1 && (
                            <div className="space-y-3">
                                <div className="flex flex-wrap items-end gap-2">
                                    <div className="space-y-1.5">
                                        <Label htmlFor="helm-diff-from" className="text-xs text-muted-foreground">{t("dialogs:helmHistory.from")}</Label>
                                        <select id="helm-diff-from" data-testid={T.pickFrom} value={from} onChange={(e) => setFrom(e.target.value)} className="h-8 rounded-md border border-border bg-muted/30 px-2 text-sm">
                                            {revs.map((r) => <option key={r.revision} value={r.revision}>{r.revision}</option>)}
                                        </select>
                                    </div>
                                    <div className="space-y-1.5">
                                        <Label htmlFor="helm-diff-to" className="text-xs text-muted-foreground">{t("dialogs:helmHistory.to")}</Label>
                                        <select id="helm-diff-to" data-testid={T.pickTo} value={to} onChange={(e) => setTo(e.target.value)} className="h-8 rounded-md border border-border bg-muted/30 px-2 text-sm">
                                            {revs.map((r) => <option key={r.revision} value={r.revision}>{r.revision}</option>)}
                                        </select>
                                    </div>
                                    <button type="button" className="btn btn-line btn-sm" data-testid={T.compare} disabled={busy || !from || !to} onClick={compare}>
                                        {t("dialogs:helmHistory.compare")}
                                    </button>
                                </div>
                                <p className="text-[11px] text-ink-3">{t("dialogs:helmHistory.redactNote")}</p>
                                {diff && (
                                    <div className="space-y-3">
                                        <DiffBlock lines={diff.values} testId={T.diffValues} title={t("dialogs:helmHistory.values")} />
                                        <DiffBlock lines={diff.manifest} testId={T.diffManifest} title={t("dialogs:helmHistory.manifest")} />
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                )}

                {preview && (
                    <div className="space-y-3" data-testid={T.preview}>
                        <p className="text-sm">{t("dialogs:helmHistory.summary", { from: preview.current, to: preview.target, added: preview.added, removed: preview.removed })}</p>
                        {preview.valuesChanged ? <p className="text-xs text-ink-3">{t("dialogs:helmHistory.valuesChanged")}</p> : null}
                        {(preview.warnings || []).map((w) => (
                            <div key={w} className="flex gap-2 rounded-xl border border-tone-red p-3 text-sm" data-testid={T.warning(w)}>
                                <AlertTriangle size={16} className="mt-0.5 shrink-0 text-tone-red" aria-hidden="true" />
                                <span>{t(`dialogs:helmHistory.warn.${w}`)}</span>
                            </div>
                        ))}
                        <DiffBlock lines={preview.manifest} testId={T.diffManifest} title={t("dialogs:helmHistory.manifest")} />
                        <div className="space-y-1.5">
                            <Label htmlFor="helm-rollback-confirm" className="text-xs text-muted-foreground">
                                <Trans i18nKey="dialogs:helmHistory.confirmLabel" values={{ name: preview.confirmName }} components={{ code: <span className="mono" /> }} />
                            </Label>
                            <Input id="helm-rollback-confirm" data-testid={T.confirmInput} value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" className="bg-muted/30" />
                        </div>
                        <DialogFooter>
                            <button type="button" className="btn btn-line" data-testid={T.back} onClick={() => setPreview(null)} disabled={busy}>
                                <ArrowLeft size={14} /> {t("dialogs:helmHistory.back")}
                            </button>
                            <button type="button" className="btn btn-danger" data-testid={T.confirm} onClick={rollback} disabled={busy || typed !== preview.confirmName}>
                                <RotateCcw size={14} /> {busy ? t("dialogs:helmHistory.rollingBack") : t("dialogs:helmHistory.rollback", { n: preview.target })}
                            </button>
                        </DialogFooter>
                    </div>
                )}
            </DialogContent>
        </Dialog>
    );
}
