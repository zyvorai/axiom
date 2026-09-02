import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ArrowLeft, Eye, EyeOff, KeySquare } from "lucide-react";
import { toast } from "sonner";
import MacTerm from "@/components/MacTerm";

export default function SecretDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [detail, setDetail] = useState(null);
    const [revealed, setRevealed] = useState(false);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        let alive = true;
        setRevealed(false);
        api
            .get(`/secrets/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}`)
            .then((r) => alive && setDetail(r.data))
            .catch(() => alive && setDetail(false));
        return () => {
            alive = false;
        };
    }, [ns, nm]);

    const toggleReveal = async () => {
        if (revealed) {
            setRevealed(false);
            return;
        }
        setBusy(true);
        try {
            const { data } = await api.get(`/secrets/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}?values=true`);
            setDetail(data);
            setRevealed(true);
        } catch (e) {
            toast.error("Reveal failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

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
                <Button variant="ghost" size="sm" onClick={() => navigate("/secrets")} className="gap-1.5">
                    <ArrowLeft className="h-3.5 w-3.5" /> Secrets
                </Button>
                <GlassCard className="p-8 text-center text-sm text-muted-foreground">
                    Secret <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    const keys = detail.keys || [];

    return (
        <div className="space-y-6">
            <DetailHeader
                eyebrow="Secret"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-primary hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <Badge variant="secondary" className="mono text-[10px]">
                            {detail.type || "Opaque"}
                        </Badge>
                        <span>·</span>
                        <span>{keys.length} keys</span>
                        <span>·</span>
                        <span>{detail.age || "—"}</span>
                    </>
                }
                action={
                    <div className="flex gap-2">
                        <Button size="sm" variant="outline" onClick={toggleReveal} disabled={busy} className="rounded-xl gap-1.5">
                            {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                            {revealed ? "Hide values" : busy ? "Loading…" : "Reveal values"}
                        </Button>
                        <Button variant="outline" size="sm" onClick={() => navigate("/secrets")} className="rounded-xl gap-1.5">
                            <ArrowLeft className="h-3.5 w-3.5" /> All secrets
                        </Button>
                    </div>
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
                    <KeySquare className="h-4 w-4 text-i17-lavender" /> Keys
                </div>
                {keys.length === 0 ? (
                    <div className="px-4 py-10 text-center text-sm text-muted-foreground">No keys.</div>
                ) : (
                    keys.map((k) => {
                        const key = typeof k === "string" ? k : k.key;
                        const size = typeof k === "object" ? k.size : null;
                        const value = revealed ? detail.values?.[key] : null;
                        return (
                            <div key={key} className="border-b px-4 py-3 last:border-none">
                                <div className="mb-1 flex items-center gap-2">
                                    <Badge variant="outline" className="mono text-[10px]">
                                        {key}
                                    </Badge>
                                    {size != null && <span className="text-[11px] text-muted-foreground">{size} bytes</span>}
                                </div>
                                <MacTerm
                                    title={key}
                                    bodyClassName="max-h-40 overflow-auto whitespace-pre-wrap px-3 py-2.5 text-[12px] leading-[1.55] text-[#f1f1f0]"
                                >
                                    {revealed ? value ?? "—" : "••••••••"}
                                </MacTerm>
                            </div>
                        );
                    })
                )}
            </GlassCard>
        </div>
    );
}
