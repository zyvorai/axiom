import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { ArrowLeft, Eye, EyeOff, KeySquare } from "lucide-react";
import { toast } from "sonner";
import { useTranslation, Trans } from "react-i18next";
import MacTerm from "@/components/MacTerm";
import { Textarea } from "@/components/ui/textarea";

export default function SecretDetail() {
    const { t } = useTranslation();
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [detail, setDetail] = useState(null);
    const [revealed, setRevealed] = useState(false);
    const [busy, setBusy] = useState(false);
    const [draft, setDraft] = useState({}); // edited values while revealed, keyed by secret key

    useEffect(() => {
        let alive = true;
        setRevealed(false);
        setDraft({});
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
            setDraft(data.values || {});
            setRevealed(true);
        } catch (e) {
            toast.error(t("toasts:secretDetail.revealFailed"), { description: e?.response?.data?.error || e.message });
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
                <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/secrets")}>
                    <ArrowLeft className="h-3.5 w-3.5" /> {t("pages:secretDetail.back")}
                </button>
                <GlassCard className="empty">
                    <Trans
                        i18nKey="pages:secretDetail.notFound"
                        values={{ ref: `${ns}/${nm}` }}
                        components={{ mono: <span className="mono text-foreground" /> }}
                    />
                </GlassCard>
            </div>
        );
    }

    const keys = detail.keys || [];
    const saved = detail.values || {};
    const dirty = revealed && Object.keys({ ...saved, ...draft }).some((k) => (draft[k] ?? "") !== (saved[k] ?? ""));

    const save = async () => {
        setBusy(true);
        try {
            await api.put(`/secrets/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}`, {
                namespace: ns,
                name: nm,
                data: draft,
            });
            setDetail((d) => ({ ...d, values: draft }));
            toast.success(t("toasts:secretDetail.updated"));
        } catch (e) {
            toast.error(t("toasts:secretDetail.updateFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="page space-y-6">
            <DetailHeader
                eyebrow="Secret"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-tone-link hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <Badge variant="secondary" className="mono text-[11px]">
                            {detail.type || "Opaque"}
                        </Badge>
                        <span>·</span>
                        <span>{t("pages:secretDetail.keys", { count: keys.length })}</span>
                        <span>·</span>
                        <span>{detail.age || "—"}</span>
                    </>
                }
                action={
                    <div className="flex gap-2">
                        <button type="button" className="btn btn-line btn-sm" onClick={toggleReveal} disabled={busy}>
                            {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                            {revealed ? t("pages:secretDetail.hideValues") : busy ? t("common:states.loading") : t("pages:secretDetail.revealValues")}
                        </button>
                        {revealed && (
                            <button
                                type="button"
                                className="btn btn-fill btn-sm"
                                disabled={busy || !dirty}
                                onClick={save}
                            >
                                {t("pages:secretDetail.saveValues")}
                            </button>
                        )}
                        <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/secrets")}>
                            <ArrowLeft className="h-3.5 w-3.5" /> {t("pages:secretDetail.all")}
                        </button>
                    </div>
                }
            />

            {detail.labels && Object.keys(detail.labels).length > 0 && (
                <GlassCard>
                    <SpecTable
                        title={t("common:fields.labels")}
                        rows={Object.entries(detail.labels).map(([k, v]) => ({ label: k, value: v, mono: true }))}
                    />
                </GlassCard>
            )}

            <GlassCard className="overflow-hidden">
                <div className="flex items-center gap-2 border-b px-4 py-3 text-sm font-medium">
                    <KeySquare className="h-4 w-4 text-tone-purple" /> {t("pages:secretDetail.keysTitle")}
                </div>
                {keys.length === 0 ? (
                    <div className="px-4 py-10 text-center text-sm text-muted-foreground">{t("pages:secretDetail.noKeys")}</div>
                ) : (
                    keys.map((k) => {
                        const key = typeof k === "string" ? k : k.key;
                        const size = typeof k === "object" ? k.size : null;
                        return (
                            <div key={key} className="border-b px-4 py-3 last:border-none">
                                <div className="mb-1 flex items-center gap-2">
                                    <Badge variant="outline" className="mono text-[11px]">
                                        {key}
                                    </Badge>
                                    {size != null && <span className="text-[11px] text-muted-foreground">{t("pages:secretDetail.bytes", { size })}</span>}
                                </div>
                                {revealed ? (
                                    <Textarea
                                        aria-label={t("pages:secretDetail.valueAria", { key })}
                                        value={draft[key] ?? ""}
                                        onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
                                        spellCheck={false}
                                        className="mono min-h-[5.5rem] text-[12px] leading-[1.55]"
                                    />
                                ) : (
                                    <MacTerm
                                        title={key}
                                        bodyClassName="max-h-40 overflow-auto whitespace-pre-wrap px-3 py-2.5 text-[12px] leading-[1.55] text-term-plain"
                                    >
                                        ••••••••
                                    </MacTerm>
                                )}
                            </div>
                        );
                    })
                )}
            </GlassCard>
        </div>
    );
}
