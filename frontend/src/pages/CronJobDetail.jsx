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

export default function CronJobDetail() {
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
            toast.success(`Triggered ${nm}`);
            await load();
        } catch (e) {
            toast.error("Trigger failed", { description: e?.response?.data?.error || e.message });
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
                    <ArrowLeft className="h-3.5 w-3.5" /> Jobs
                </button>
                <GlassCard className="empty">
                    CronJob <span className="mono text-foreground">{ns}/{nm}</span> was not found.
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
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-primary hover:underline">
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
                            <PlayCircle className="h-3.5 w-3.5" /> {busy ? "Triggering…" : "Run now"}
                        </button>
                        <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/jobs")}>
                            <ArrowLeft className="h-3.5 w-3.5" /> All jobs
                        </button>
                    </div>
                }
            />

            <GlassCard>
                <SpecTable
                    title="Schedule"
                    rows={[
                        { label: "Cron", value: cron.schedule || "—", mono: true },
                        { label: "Suspended", value: cron.suspend ? "Yes" : "No" },
                        { label: "Active jobs", value: String(cron.active ?? 0), mono: true },
                        { label: "Last schedule", value: cron.lastSchedule || "—", mono: true },
                    ]}
                />
            </GlassCard>

            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 style={{ fontSize: 15 }}>Recent jobs ({ownedJobs.length})</h2>
                </div>
                {ownedJobs.length === 0 ? (
                    <div className="empty">No jobs from this CronJob yet.</div>
                ) : (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Status</th>
                                <th>Completions</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {ownedJobs.map((j) => (
                                <tr
                                    key={j.name}
                                    style={{ cursor: "pointer" }}
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
