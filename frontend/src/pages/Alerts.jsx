import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { formatDistanceToNowStrict } from "date-fns";
import { useTranslation } from "react-i18next";
import { dateFnsLocale } from "@/lib/i18n";
import { AlertTriangle, BellOff, Plus } from "lucide-react";
import { resourceHref } from "@/lib/resourceHref";
import { RunAgentButton } from "@/components/RunAgentButton";
import { toast } from "sonner";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return iso;
    }
}

function alertTarget(a) {
    const labels = a.labels || {};
    const ns = labels.namespace || labels.exported_namespace || "default";
    const pod = labels.pod || labels.pod_name;
    const name = pod || labels.instance || labels.alertname || a.fingerprint || "alert";
    const kind = pod ? "Pod" : "Namespace";
    return {
        namespace: ns,
        kind,
        name: pod || ns,
        goal: `Investigate firing alert ${labels.alertname || name}: ${a.summary || a.annotations?.description || a.annotations?.summary || ""}`.trim(),
        reason: labels.alertname || a.summary || "firing alert",
    };
}

export default function Alerts() {
    const { t } = useTranslation();
    const [d, setD] = useState(null);
    const [am, setAm] = useState(null);
    const [silences, setSilences] = useState(null);
    const [silenceForm, setSilenceForm] = useState({ matchers: 'alertname="Watchdog"', comment: "", hours: "2" });
    const [busy, setBusy] = useState("");

    const loadAM = () => {
        api.get("/alertmanager/alerts").then((r) => setAm(r.data)).catch(() => setAm({ available: false, alerts: [] }));
        api.get("/alertmanager/silences").then((r) => setSilences(r.data)).catch(() => setSilences({ available: false, silences: [] }));
    };

    useEffect(() => {
        let alive = true;
        const load = () => {
            api.get("/alerts").then((r) => alive && setD(r.data));
            if (alive) loadAM();
        };
        load();
        const timer = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const createSilence = async () => {
        setBusy("silence");
        try {
            const matchers = [];
            for (const part of String(silenceForm.matchers || "").split(",")) {
                const m = part.trim().match(/^([^=!~]+)(=|!=|=~|!~)"?(.*?)"?$/);
                if (!m) continue;
                matchers.push({
                    name: m[1].trim(),
                    value: m[3],
                    isRegex: m[2].includes("~"),
                    isEqual: !m[2].startsWith("!"),
                });
            }
            if (matchers.length === 0) throw new Error(t("pages:alerts.silenceBadMatchers"));
            const hours = Math.max(1, Number(silenceForm.hours) || 2);
            const startsAt = new Date().toISOString();
            const endsAt = new Date(Date.now() + hours * 3600 * 1000).toISOString();
            await api.post("/alertmanager/silences", {
                matchers,
                startsAt,
                endsAt,
                comment: silenceForm.comment || "Silenced from Axiom",
            });
            toast.success(t("pages:alerts.silenceCreated"));
            setSilenceForm((f) => ({ ...f, comment: "" }));
            loadAM();
        } catch (e) {
            toast.error(t("pages:alerts.silenceFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const expireSilence = async (id) => {
        setBusy(`expire-${id}`);
        try {
            await api.delete(`/alertmanager/silences/${encodeURIComponent(id)}`);
            toast.success(t("pages:alerts.silenceExpired"));
            loadAM();
        } catch (e) {
            toast.error(t("pages:alerts.silenceFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!d) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    const warnings = d.warnings || [];
    const rules = d.rules || [];
    const firing = am?.alerts || [];

    return (
        <div className="page">
            <PageHeader
                title={t("pages:alerts.title")}
                description={t("pages:alerts.description")}
            />

            {am && (
                <GlassCard className="overflow-hidden p-0">
                    <div className="panel-h flex items-center justify-between gap-2">
                        <span>{t("pages:alerts.firingTitle")}</span>
                        {!am.available && <span className="text-[11px] font-normal text-ink-3">{am.note || t("pages:alerts.amNotConfigured")}</span>}
                        {am.error && <span className="text-[11px] font-normal text-tone-orange">{am.error}</span>}
                    </div>
                    <div className="max-h-[400px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:alerts.firingTitle")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:alerts.columns.alert")}</th>
                                    <th>{t("pages:alerts.columns.severity")}</th>
                                    <th>{t("common:fields.time")}</th>
                                    <th><span className="sr-only">{t("pages:ops.agents.runAgent")}</span></th>
                                </tr>
                            </thead>
                            <tbody>
                                {firing.map((a, i) => {
                                    const labels = a.labels || {};
                                    return (
                                        <tr key={a.fingerprint || i}>
                                            <td>
                                                <span className="mono text-tone-link">{labels.alertname || a.summary || "alert"}</span>
                                                <div className="text-[12px] text-ink-3">{a.summary || a.annotations?.description || a.annotations?.summary || ""}</div>
                                                {labels.namespace && (
                                                    <div className="mono text-[11px] text-ink-3">{labels.namespace}{labels.pod ? `/${labels.pod}` : ""}</div>
                                                )}
                                            </td>
                                            <td className="mono">{labels.severity || a.status || "—"}</td>
                                            <td className="num">{a.startsAt ? relTime(a.startsAt) : "—"}</td>
                                            <td>
                                                <RunAgentButton target={alertTarget(a)} testId={`alert-run-agent-${i}`} />
                                            </td>
                                        </tr>
                                    );
                                })}
                                {am.available && firing.length === 0 && (
                                    <tr>
                                        <td colSpan={4}>
                                            <div className="empty">{t("pages:alerts.noFiring")}</div>
                                        </td>
                                    </tr>
                                )}
                                {!am.available && (
                                    <tr>
                                        <td colSpan={4}>
                                            <div className="empty">{t("pages:alerts.amNotConfigured")}</div>
                                        </td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>
            )}

            {silences?.available && (
                <GlassCard className="space-y-3 p-4">
                    <div className="flex items-center gap-2 text-sm font-medium">
                        <BellOff size={14} /> {t("pages:alerts.silencesTitle")}
                    </div>
                    <div className="grid gap-2 sm:grid-cols-3">
                        <Input
                            className="bg-muted/30 sm:col-span-2"
                            value={silenceForm.matchers}
                            onChange={(e) => setSilenceForm((f) => ({ ...f, matchers: e.target.value }))}
                            placeholder='alertname="HighCPU",namespace="default"'
                            aria-label={t("pages:alerts.silenceMatchers")}
                        />
                        <Input
                            className="bg-muted/30"
                            value={silenceForm.hours}
                            onChange={(e) => setSilenceForm((f) => ({ ...f, hours: e.target.value }))}
                            placeholder="2"
                            aria-label={t("pages:alerts.silenceHours")}
                        />
                        <Input
                            className="bg-muted/30 sm:col-span-2"
                            value={silenceForm.comment}
                            onChange={(e) => setSilenceForm((f) => ({ ...f, comment: e.target.value }))}
                            placeholder={t("pages:alerts.silenceComment")}
                        />
                        <button type="button" className="btn btn-line btn-sm" disabled={busy === "silence"} onClick={createSilence}>
                            <Plus size={13} /> {busy === "silence" ? t("common:actions.saving") : t("pages:alerts.createSilence")}
                        </button>
                    </div>
                    <div className="max-h-[240px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:alerts.silencesTitle")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:alerts.columns.matchers")}</th>
                                    <th>{t("common:fields.status")}</th>
                                    <th>{t("pages:alerts.columns.ends")}</th>
                                    <th />
                                </tr>
                            </thead>
                            <tbody>
                                {(silences.silences || []).map((s) => (
                                    <tr key={s.id}>
                                        <td className="mono text-[12px]">
                                            {(s.matchers || []).map((m) => `${m.name}${m.isEqual === false ? "!" : ""}${m.isRegex ? "~" : "="}${m.value}`).join(", ") || s.id}
                                            {s.comment ? <div className="text-ink-3 font-sans">{s.comment}</div> : null}
                                        </td>
                                        <td>{s.status || "—"}</td>
                                        <td className="num">{s.endsAt ? relTime(s.endsAt) : "—"}</td>
                                        <td>
                                            {String(s.status || "").toLowerCase() !== "expired" && (
                                                <button
                                                    type="button"
                                                    className="btn btn-line btn-sm"
                                                    disabled={busy === `expire-${s.id}`}
                                                    onClick={() => expireSilence(s.id)}
                                                >
                                                    {t("pages:alerts.expireSilence")}
                                                </button>
                                            )}
                                        </td>
                                    </tr>
                                ))}
                                {(silences.silences || []).length === 0 && (
                                    <tr>
                                        <td colSpan={4}>
                                            <div className="empty">{t("pages:alerts.noSilences")}</div>
                                        </td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>
            )}

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">{t("pages:alerts.activeWarnings")}</div>
                <div className="max-h-[400px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:alerts.activeWarnings")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:alerts.columns.warning")}</th>
                                <th>{t("pages:alerts.columns.resource")}</th>
                                <th>{t("common:fields.count")}</th>
                                <th>{t("common:fields.time")}</th>
                                <th><span className="sr-only">{t("pages:ops.agents.runAgent")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {warnings.map((w, i) => {
                                const href = resourceHref(w.namespace, w.object);
                                const objectName = String(w.object || "").includes("/")
                                    ? String(w.object).split("/").pop()
                                    : w.object;
                                return (
                                    <tr key={i}>
                                        <td>
                                            <span className="flex items-start gap-2">
                                                <AlertTriangle size={14} className="text-tone-orange mt-0.5 shrink-0" />
                                                <span>
                                                    <span className="mono text-tone-link">{w.reason}</span> — {w.message}
                                                </span>
                                            </span>
                                        </td>
                                        <td className="mono">
                                            {href ? (
                                                <Link to={href} className="text-tone-link">
                                                    {w.namespace}/{w.object}
                                                </Link>
                                            ) : (
                                                `${w.namespace}/${w.object}`
                                            )}
                                        </td>
                                        <td className="num">×{w.count}</td>
                                        <td className="num">{relTime(w.time)}</td>
                                        <td>
                                            <RunAgentButton
                                                target={{
                                                    namespace: w.namespace || "default",
                                                    kind: "event",
                                                    name: objectName || w.object,
                                                    reason: w.reason,
                                                    goal: `Diagnose Warning ${w.reason} on ${w.namespace}/${w.object}`,
                                                }}
                                                testId={`warning-run-agent-${i}`}
                                            />
                                        </td>
                                    </tr>
                                );
                            })}
                            {warnings.length === 0 && (
                                <tr>
                                    <td colSpan={5}>
                                        <div className="empty">{t("pages:alerts.noWarnings")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">{t("pages:alerts.ruleGroups")}</div>
                <div className="max-h-[300px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:alerts.ruleGroups")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("pages:alerts.columns.groups")}</th>
                                <th>{t("common:fields.age")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rules.map((r, i) => (
                                <tr key={i}>
                                    <td className="name mono">{r.name}</td>
                                    <td className="mono">{r.namespace}</td>
                                    <td className="num">{t("pages:alerts.groupsCount", { n: r.groups })}</td>
                                    <td className="num">{r.age}</td>
                                </tr>
                            ))}
                            {rules.length === 0 && (
                                <tr>
                                    <td colSpan={4}>
                                        <div className="empty">{t("pages:alerts.noRules")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>
        </div>
    );
}
