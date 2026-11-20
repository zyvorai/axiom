import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Layers, Boxes } from "lucide-react";
import { useTranslation } from "react-i18next";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";
import CodeEditor from "@/components/CodeEditor";
import { breakable } from "@/lib/text";

function InstanceDetail({ crd, instance, onOpenChange }) {
    if (!instance) return null;
    const name = instance.metadata?.name || instance.name || "—";
    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="max-w-2xl">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">
                        {crd.kind} · {name}
                    </DialogTitle>
                </DialogHeader>
                <CodeEditor value={JSON.stringify(instance, null, 2)} editable={false} className="min-h-0 [&_.code-pane]:min-h-[50vh] [&_.code-pane]:max-h-[50vh]" />
            </DialogContent>
        </Dialog>
    );
}

export default function CustomResources() {
    const { t } = useTranslation();
    const [crds, setCrds] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();
    // the open CRD lives in the URL too (?crd=group/version/plural) so a deep link reopens it
    const [crdKey, setCrdKey] = useUrlState("crd", "");
    const [instances, setInstances] = useState(null);
    const [instancesError, setInstancesError] = useState(null);
    const [detail, setDetail] = useState(null);

    useEffect(() => {
        api.get("/crds").then((r) => setCrds(r.data || []));
    }, []);

    const selected = useMemo(
        () => (crds || []).find((c) => `${c.group}/${c.version}/${c.plural}` === crdKey) || null,
        [crds, crdKey]
    );

    useEffect(() => {
        if (!selected) return;
        let alive = true;
        setInstances(null);
        setInstancesError(null);
        api
            .get(`/crds/${selected.group}/${selected.version}/${selected.plural}`)
            .then((r) => alive && setInstances(r.data || []))
            .catch((e) => {
                if (!alive) return;
                setInstances([]);
                setInstancesError(e?.response?.data?.error || e.message);
            });
        return () => {
            alive = false;
        };
    }, [selected]);

    const filtered = useMemo(() => {
        if (!crds) return [];
        if (!q) return crds;
        const needle = q.toLowerCase();
        return crds.filter((c) => c.kind.toLowerCase().includes(needle) || c.group.toLowerCase().includes(needle));
    }, [crds, q]);

    const shownInstances = useMemo(() => {
        if (!instances) return instances;
        return ns === "all" ? instances : instances.filter((r) => (r.metadata?.namespace || r.namespace) === ns);
    }, [instances, ns]);
    const namespaces = useMemo(
        () => [...new Set([...knownNamespaces, ...(instances || []).map((r) => r.metadata?.namespace || r.namespace).filter(Boolean)])].sort(),
        [knownNamespaces, instances]
    );

    const openInstance = (row) => {
        const ns = row.metadata?.namespace || row.namespace;
        const name = row.metadata?.name || row.name;
        if (!selected || !name) return;
        api
            .get(`/crds/${selected.group}/${selected.version}/${selected.plural}/${encodeURIComponent(name)}`, {
                params: ns ? { namespace: ns } : {},
            })
            .then((r) => setDetail(r.data))
            .catch(() => setDetail(row));
    };

    if (!crds) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title={t("pages:customResources.title")}
                description={t("pages:customResources.description")}
                tools={
                    <>
                        <SearchBox placeholder={t("pages:customResources.searchPlaceholder")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <SavedViews
                            pageKey="custom-resources"
                            filters={{ q, ns, crd: crdKey }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                                setCrdKey(f.crd ?? "");
                            }}
                        />
                    </>
                }
            />

            <div className="grid gap-4 lg:grid-cols-[minmax(360px,36%)_minmax(0,1fr)]">
                <GlassCard className="overflow-hidden p-0">
                    <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:customResources.tableAria")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.kind")}</th>
                                    <th>{t("pages:customResources.columns.groupVersion")}</th>
                                    <th>{t("pages:customResources.columns.scope")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {filtered.length === 0 && (
                                    <tr>
                                        <td colSpan={3}>
                                            <div className="empty">{t("pages:customResources.noMatch")}</div>
                                        </td>
                                    </tr>
                                )}
                                {filtered.map((c) => {
                                    const active = selected && selected.group === c.group && selected.plural === c.plural;
                                    return (
                                        <tr
                                            key={`${c.group}/${c.version}/${c.plural}`}
                                            onClick={() => {
                                                setCrdKey(`${c.group}/${c.version}/${c.plural}`);
                                                setDetail(null);
                                            }}
                                            className={active ? "cursor-pointer bg-[var(--active)]" : "cursor-pointer"}
                                        >
                                            <td className="name mono">
                                                <span className="flex items-center gap-2">
                                                    <Layers size={14} className="text-ink-3" />
                                                    {c.kind}
                                                </span>
                                            </td>
                                            <td className="mono num wrap">
                                                {breakable(`${c.group}/${c.version}`)}
                                            </td>
                                            <td>
                                                <span className="pill soft">{c.scope}</span>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>

                {selected ? (
                    <AutoTable
                        title={t("pages:customResources.instances", { kind: selected.kind })}
                        icon={Boxes}
                        items={shownInstances}
                        emptyHint={instances === null ? t("common:states.loading") : instancesError ? t("pages:customResources.permissionDenied", { error: instancesError }) : t("pages:customResources.noInstances")}
                        onRowClick={openInstance}
                    />
                ) : (
                    <GlassCard>
                        <div className="empty">{t("pages:customResources.selectHint")}</div>
                    </GlassCard>
                )}
            </div>

            {detail && <InstanceDetail crd={selected} instance={detail} onOpenChange={(o) => !o && setDetail(null)} />}
        </div>
    );
}
