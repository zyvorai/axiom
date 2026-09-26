import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { ArrowLeft } from "lucide-react";

function isJobPod(pod, jobName) {
    const labels = pod.labels || {};
    return labels["job-name"] === jobName || labels["batch.kubernetes.io/job-name"] === jobName || (pod.name || "").startsWith(`${jobName}-`);
}

export default function JobDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [job, setJob] = useState(null);
    const [pods, setPods] = useState([]);

    useEffect(() => {
        let alive = true;
        const q = `?namespace=${encodeURIComponent(ns)}`;
        Promise.all([
            api.get(`/jobs${q}`).catch(() => ({ data: [] })),
            api.get(`/pods${q}`).catch(() => ({ data: [] })),
        ]).then(([jRes, pRes]) => {
            if (!alive) return;
            const found = (jRes.data || []).find((j) => j.name === nm);
            setJob(found || false);
            setPods(pRes.data || []);
        });
        return () => {
            alive = false;
        };
    }, [ns, nm]);

    const jobPods = useMemo(() => pods.filter((p) => isJobPod(p, nm)), [pods, nm]);

    if (job === null) {
        return (
            <div className="page space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (job === false) {
        return (
            <div className="page space-y-4">
                <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/jobs")}>
                    <ArrowLeft className="h-3.5 w-3.5" /> Jobs
                </button>
                <GlassCard className="empty">
                    Job <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    return (
        <div className="page space-y-6">
            <DetailHeader
                eyebrow="Job"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-tone-link hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <StatusBadge status={job.status} />
                        <span>·</span>
                        <span>{job.age || "—"}</span>
                    </>
                }
                action={
                    <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/jobs")}>
                        <ArrowLeft className="h-3.5 w-3.5" /> All jobs
                    </button>
                }
            />

            <GlassCard>
                <SpecTable
                    title="Summary"
                    rows={[
                        { label: "Completions", value: job.completions || "—", mono: true },
                        { label: "Duration", value: job.duration || "—", mono: true },
                        {
                            label: "Owner CronJob",
                            value: job.ownerCronJob ? (
                                <Link
                                    to={`/cronjobs/${encodeURIComponent(ns)}/${encodeURIComponent(job.ownerCronJob)}`}
                                    className="text-tone-link hover:underline"
                                >
                                    {job.ownerCronJob}
                                </Link>
                            ) : (
                                "—"
                            ),
                            mono: true,
                        },
                    ]}
                />
            </GlassCard>

            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="text-[15px]">Pods ({jobPods.length})</h2>
                </div>
                {jobPods.length === 0 ? (
                    <div className="empty">No pods found for this job.</div>
                ) : (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Status</th>
                                <th>Restarts</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {jobPods.map((p) => (
                                <tr
                                    key={p.name} className="cursor-pointer"
                                    onClick={() => navigate(`/pods/${encodeURIComponent(ns)}/${encodeURIComponent(p.name)}`)}
                                >
                                    <td className="name mono">{p.name}</td>
                                    <td>
                                        <StatusBadge status={p.status} />
                                    </td>
                                    <td className="num">{p.restarts ?? 0}</td>
                                    <td className="num">{p.age || "—"}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </GlassCard>
        </div>
    );
}
