import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNowStrict } from "date-fns";
import { AlertTriangle } from "lucide-react";
import { resourceHref } from "@/lib/resourceHref";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
    } catch {
        return iso;
    }
}

export default function Alerts() {
    const [d, setD] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/alerts").then((r) => alive && setD(r.data));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

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

    return (
        <div className="page">
            <PageHeader
                title="Alerts"
                description="Active warnings and Prometheus alerting rule groups."
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">Active warnings</div>
                <div className="max-h-[400px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label="Active warnings">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Warning</th>
                                <th>Resource</th>
                                <th>Count</th>
                                <th>Time</th>
                            </tr>
                        </thead>
                        <tbody>
                            {warnings.map((w, i) => {
                                const href = resourceHref(w.namespace, w.object);
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
                                    </tr>
                                );
                            })}
                            {warnings.length === 0 && (
                                <tr>
                                    <td colSpan={4}>
                                        <div className="empty">No active warnings.</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">Alerting rule groups</div>
                <div className="max-h-[300px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label="Alerting rule groups">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Namespace</th>
                                <th>Groups</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rules.map((r, i) => (
                                <tr key={i}>
                                    <td className="name mono">{r.name}</td>
                                    <td className="mono">{r.namespace}</td>
                                    <td className="num">{r.groups} groups</td>
                                    <td className="num">{r.age}</td>
                                </tr>
                            ))}
                            {rules.length === 0 && (
                                <tr>
                                    <td colSpan={4}>
                                        <div className="empty">No alerting rules configured.</div>
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
