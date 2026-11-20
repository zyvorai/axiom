import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { ArrowLeft, PlayCircle } from "lucide-react";
import { toast } from "sonner";
import { useTranslation, Trans } from "react-i18next";

export default function CronJobDetail() {
    const { t } = useTranslation();
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [cron, setCron] = useState(null);
    const [jobs, setJobs] = useState([]);
    const [busy, setBusy] = useState(false);

    const load = () => {
        const q = `?namespace=${encodeURIComponent(ns)}`;
        return Promise.all([
            api.get(`/cronjobs${q}`).catch(() => ({ data: [] })),
            api.get(`/jobs${q}`).catch(() => ({ data: [] })),
        ]).then(([cRes, jRes]) => {
            const found = (cRes.data || []).find((c) => c.name === nm);
            setCron(found || false);
            setJobs(jRes.data || []);
        });
    };

    useEffect(() => {
        let alive = true;
        const q = `?namespace=${encodeURIComponent(ns)}`;
        Promise.all([
            api.get(`/cronjobs${q}`).catch(() => ({ data: [] })),
            api.get(`/jobs${q}`).catch(() => ({ data: [] })),
        ]).then(([cRes, jRes]) => {
            if (!alive) return;
            const found = (cRes.data || []).find((c) => c.name === nm);
            setCron(found || false);
            setJobs(jRes.data || []);
        });
        return () => {
            alive = false;
        };
    }, [ns, nm]);

    const ownedJobs = useMemo(
        () => (jobs || []).filter((j) => j.ownerCronJob === nm || (j.name || "").startsWith(`${nm}-`)),
        [jobs, nm]
    );

    const trigger = async () => {
        setBusy(true);
        try {
            await api.post(`/cronjobs/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}/trigger`);
            toast.success(t("toasts:cronJobDetail.triggered", { name: nm }));
            await load();
        } catch (e) {
            toast.error(t("toasts:cronJobDetail.triggerFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    if (cron === null) {
        return (
            <div className="page space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (cron === false) {
        return (
            <div className="page space-y-4">
                <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/jobs")}>
                    <ArrowLeft className="h-3.5 w-3.5" /> {t("pages:cronJobDetail.backToJobs")}
                </button>
                <GlassCard className="empty">
                    <Trans i18nKey="pages:cronJobDetail.notFound" values={{ name: `${ns}/${nm}` }} components={{ code: <span className="mono text-foreground" /> }} />
                </GlassCard>
            </div>
        );
    }

    return (
        <div className="page space-y-6">
            <DetailHeader
                eyebrow="CronJob"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-tone-link hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <StatusBadge status={cron.suspend ? "Paused" : "Running"} />
                        <span>·</span>
                        <span className="mono">{cron.schedule}</span>
                        <span>·</span>
                        <span>{cron.age || "—"}</span>
                    </>
                }
                action={
                    <div className="flex gap-2">
                        <button type="button" className="btn btn-line btn-sm" disabled={busy} onClick={trigger}>
                            <PlayCircle className="h-3.5 w-3.5" /> {busy ? t("pages:cronJobDetail.triggering") : t("pages:cronJobDetail.runNow")}
                        </button>
                        <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/jobs")}>
                            <ArrowLeft className="h-3.5 w-3.5" /> {t("pages:cronJobDetail.allJobs")}
                        </button>
                    </div>
                }
            />

            <GlassCard>
                <SpecTable
                    title={t("pages:cronJobDetail.schedule")}
                    rows={[
                        { label: t("pages:cronJobDetail.cron"), value: cron.schedule || "—", mono: true },
                        { label: t("pages:cronJobDetail.suspended"), value: cron.suspend ? t("common:states.yes") : t("common:states.no") },
                        { label: t("pages:cronJobDetail.activeJobs"), value: String(cron.active ?? 0), mono: true },
                        { label: t("pages:cronJobDetail.lastSchedule"), value: cron.lastSchedule || "—", mono: true },
                    ]}
                />
            </GlassCard>

            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="text-[15px]">{t("pages:cronJobDetail.recentJobs", { n: ownedJobs.length })}</h2>
                </div>
                {ownedJobs.length === 0 ? (
                    <div className="empty">{t("pages:cronJobDetail.noJobs")}</div>
                ) : (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.status")}</th>
                                <th>{t("pages:cronJobDetail.completions")}</th>
                                <th>{t("common:fields.age")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {ownedJobs.map((j) => (
                                <tr
                                    key={j.name} className="cursor-pointer"
                                    onClick={() => navigate(`/jobs/${encodeURIComponent(ns)}/${encodeURIComponent(j.name)}`)}
                                >
                                    <td className="name mono">{j.name}</td>
                                    <td>
                                        <StatusBadge status={j.status} />
                                    </td>
                                    <td className="num">{j.completions || "—"}</td>
                                    <td className="num">{j.age || "—"}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </GlassCard>
        </div>
    );
}
