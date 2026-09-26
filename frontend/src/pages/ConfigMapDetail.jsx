import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { ArrowLeft, FileCog, Save } from "lucide-react";
import MacTerm from "@/components/MacTerm";
import { toast } from "sonner";

export default function ConfigMapDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [detail, setDetail] = useState(null);
    const [editing, setEditing] = useState(false);
    const [draft, setDraft] = useState("");
    const [busy, setBusy] = useState(false);

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

    const startEdit = () => {
        setDraft(JSON.stringify(detail.data || {}, null, 2));
        setEditing(true);
    };

    const save = async () => {
        setBusy(true);
        try {
            const data = JSON.parse(draft);
            const { data: updated } = await api.put(`/configmaps/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}`, {
                namespace: ns,
                name: nm,
                data,
            });
            setDetail(updated);
            setEditing(false);
            toast.success("ConfigMap updated");
        } catch (e) {
            toast.error("Update failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    if (detail === null) {
        return (
            <div className="page space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (detail === false) {
        return (
            <div className="page space-y-4">
                <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/configmaps")}>
                    <ArrowLeft className="h-3.5 w-3.5" /> ConfigMaps
                </button>
                <GlassCard className="empty">
                    ConfigMap <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    const entries = Object.entries(detail.data || {});

    return (
        <div className="page space-y-6">
            <DetailHeader
                eyebrow="ConfigMap"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-tone-link hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <span>{entries.length} keys</span>
                        <span>·</span>
                        <span>{detail.age || "—"}</span>
                    </>
                }
                action={
                    <div className="flex gap-2">
                        {!editing ? (
                            <button type="button" className="btn btn-line btn-sm" onClick={startEdit}>
                                Edit data
                            </button>
                        ) : (
                            <button type="button" className="btn btn-fill btn-sm" onClick={save} disabled={busy}>
                                <Save className="h-3.5 w-3.5" /> {busy ? "Saving…" : "Save"}
                            </button>
                        )}
                        <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/configmaps")}>
                            <ArrowLeft className="h-3.5 w-3.5" /> All ConfigMaps
                        </button>
                    </div>
                }
            />

            {detail.labels && Object.keys(detail.labels).length > 0 && (
                <GlassCard>
                    <SpecTable
                        title="Labels"
                        rows={Object.entries(detail.labels).map(([k, v]) => ({ label: k, value: v, mono: true }))}
                    />
                </GlassCard>
            )}

            <GlassCard className="overflow-hidden">
                <div className="flex items-center gap-2 border-b px-4 py-3 text-sm font-medium">
                    <FileCog className="h-4 w-4 text-tone-link" /> Data
                </div>
                {editing ? (
                    <div className="p-4">
                        <Textarea className="mono min-h-[280px] bg-muted/30 text-xs" value={draft} onChange={(e) => setDraft(e.target.value)} />
                    </div>
                ) : entries.length === 0 ? (
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
                                bodyClassName="max-h-48 overflow-auto whitespace-pre-wrap px-3 py-2.5 text-[12px] leading-[1.55] text-term-plain"
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
