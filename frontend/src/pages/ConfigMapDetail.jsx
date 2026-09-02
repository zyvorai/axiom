import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ArrowLeft, FileCog } from "lucide-react";
import MacTerm from "@/components/MacTerm";

export default function ConfigMapDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [detail, setDetail] = useState(null);

    useEffect(() => {
        let alive = true;
        api
            .get(`/configmaps/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}`)
            .then((r) => alive && setDetail(r.data))
            .catch(() => alive && setDetail(false));
        return () => {
            alive = false;
        };
    }, [ns, nm]);

    if (detail === null) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (detail === false) {
        return (
            <div className="space-y-4">
                <Button variant="ghost" size="sm" onClick={() => navigate("/configmaps")} className="gap-1.5">
                    <ArrowLeft className="h-3.5 w-3.5" /> ConfigMaps
                </Button>
                <GlassCard className="p-8 text-center text-sm text-muted-foreground">
                    ConfigMap <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    const entries = Object.entries(detail.data || {});

    return (
        <div className="space-y-6">
            <DetailHeader
                eyebrow="ConfigMap"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-primary hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <span>{entries.length} keys</span>
                        <span>·</span>
                        <span>{detail.age || "—"}</span>
                    </>
                }
                action={
                    <Button variant="outline" size="sm" onClick={() => navigate("/configmaps")} className="rounded-xl gap-1.5">
                        <ArrowLeft className="h-3.5 w-3.5" /> All ConfigMaps
                    </Button>
                }
            />

            {detail.labels && Object.keys(detail.labels).length > 0 && (
                <SpecTable
                    title="Labels"
                    rows={Object.entries(detail.labels).map(([k, v]) => ({ label: k, value: v, mono: true }))}
                />
            )}

            <GlassCard className="overflow-hidden">
                <div className="flex items-center gap-2 border-b px-4 py-3 text-sm font-medium">
                    <FileCog className="h-4 w-4 text-i17-sky" /> Data
                </div>
                {entries.length === 0 ? (
                    <div className="px-4 py-10 text-center text-sm text-muted-foreground">No data keys.</div>
                ) : (
                    entries.map(([key, value]) => (
                        <div key={key} className="border-b px-4 py-3 last:border-none">
                            <div className="mb-1">
                                <Badge variant="outline" className="mono text-[10px]">
                                    {key}
                                </Badge>
                            </div>
                            <MacTerm
                                title={key}
                                bodyClassName="max-h-48 overflow-auto whitespace-pre-wrap px-3 py-2.5 text-[12px] leading-[1.55] text-[#f1f1f0]"
                            >
                                {value}
                            </MacTerm>
                        </div>
                    ))
                )}
            </GlassCard>
        </div>
    );
}
