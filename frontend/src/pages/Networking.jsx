import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import ApplyManifestDialog from "@/components/ApplyManifestDialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Network, Boxes, ArrowRightLeft, Radar, ShieldHalf, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { NET } from "@/constants/testIds";
import { useConfirm } from "@/components/ConfirmDialog";
import CodeEditor from "@/components/CodeEditor";

const NETPOL_TEMPLATE = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: example-policy
  namespace: default
spec:
  podSelector: {}
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
`;

function NetworkPolicyPanel() {
    const confirm = useConfirm();
    const [policies, setPolicies] = useState(null);
    const [createOpen, setCreateOpen] = useState(false);

    const load = () => api.get("/networkpolicies").then((r) => setPolicies(r.data || []));

    useEffect(() => {
        let alive = true;
        api.get("/networkpolicies").then((r) => alive && setPolicies(r.data || []));
        return () => {
            alive = false;
        };
    }, []);

    const remove = async (p) => {
        const ok = await confirm({
            title: `Delete NetworkPolicy ${p.name}?`,
            description: "This may remove traffic restrictions currently enforced by this policy. This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/networkpolicies/${p.namespace}/${encodeURIComponent(p.name)}`);
            toast.success(`Deleted ${p.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (!policies) return <Skeleton className="h-40 rounded-2xl bg-muted" />;

    return (
        <GlassCard className="overflow-hidden">
            <div className="card-h">
                <h2 style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 15 }}>
                    <ShieldHalf size={16} style={{ color: "var(--blue)" }} /> NetworkPolicies
                </h2>
                <button type="button" className="btn btn-line btn-sm r" onClick={() => setCreateOpen(true)}>
                    <Plus size={13} /> Create via YAML
                </button>
            </div>
            {policies.length === 0 ? (
                <div className="empty">No NetworkPolicies configured.</div>
            ) : (
                <div style={{ overflowX: "auto" }} className="max-h-[360px] overflow-y-auto scrollbar-thin">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Types</th>
                                <th>Pod selector</th>
                                <th />
                            </tr>
                        </thead>
                        <tbody>
                            {policies.map((p, i) => (
                                <tr key={i} data-testid={NET.policyRow(p.name)}>
                                    <td className="name">
                                        {p.name}
                                        <small>{p.namespace}</small>
                                    </td>
                                    <td>{(p.policyTypes || []).join(", ") || "—"}</td>
                                    <td className="mono">{p.podSelector || "all pods"}</td>
                                    <td>
                                        <button type="button" className="btn btn-line btn-sm" onClick={() => remove(p)}>
                                            <Trash2 size={13} />
                                        </button>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            <ApplyManifestDialog
                open={createOpen}
                onOpenChange={setCreateOpen}
                title="Create NetworkPolicy"
                initialManifest={NETPOL_TEMPLATE}
                onApplied={() => {
                    setCreateOpen(false);
                    load();
                }}
            />
        </GlassCard>
    );
}

function flowItems(flows) {
    if (!flows) return null;
    if (Array.isArray(flows)) return flows;
    if (Array.isArray(flows.items)) return flows.items;
    if (Array.isArray(flows.flows)) return flows.flows;
    return null;
}

function PacketWolfPanel() {
    const [status, setStatus] = useState(null);
    const [flows, setFlows] = useState(null);
    const [flowsError, setFlowsError] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/packetwolf/status").then((r) => alive && setStatus(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    useEffect(() => {
        if (!status?.reachable) return;
        let alive = true;
        const load = () =>
            api
                .get("/packetwolf/proxy/api/v1/flows", { params: { limit: 25 } })
                .then((r) => alive && (setFlows(r.data), setFlowsError(null)))
                .catch((e) => alive && setFlowsError(e?.response?.data?.error || e.message));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [status?.reachable]);

    if (!status) return <Skeleton className="h-24 rounded-2xl bg-muted" />;

    const tone = status.reachable ? "ok" : status.configured ? "warn" : "info";
    const label = status.reachable ? "Connected" : status.configured ? "Unreachable" : "Not configured";
    const items = flowItems(flows);

    return (
        <GlassCard className="overflow-hidden">
            <div className="card-h">
                <h2 style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 15 }}>
                    <Radar size={16} style={{ color: "var(--blue)" }} /> PacketWolf
                </h2>
                <span className="r" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12.5, color: "var(--ink-3)" }}>
                    <span className={`status-dot ${tone}`} />
                    {label}
                </span>
            </div>
            <p style={{ fontSize: 13, color: "var(--ink-2)", marginBottom: 14 }}>
                {status.message || status.baseUrl || "eBPF flow visibility via the PacketWolf integration."}
            </p>
            {status.reachable && (
                <>
                    {flowsError && <div className="empty">{flowsError}</div>}
                    {!flowsError && items && items.length > 0 && (
                        <div style={{ overflowX: "auto" }} className="max-h-[360px] overflow-y-auto scrollbar-thin">
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>Source</th>
                                        <th>Destination</th>
                                        <th>Proto</th>
                                        <th>Verdict</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {items.slice(0, 25).map((f, i) => (
                                        <tr key={i} data-testid={NET.flowRow(i)}>
                                            <td className="mono">{String(f.source ?? f.src ?? f.from ?? "—")}</td>
                                            <td className="mono">{String(f.destination ?? f.dst ?? f.to ?? "—")}</td>
                                            <td>{String(f.protocol ?? f.proto ?? "—")}</td>
                                            <td>{String(f.verdict ?? f.action ?? f.disposition ?? "—")}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                    {!flowsError && items && items.length === 0 && (
                        <div className="empty">No flows in the current window.</div>
                    )}
                    {!flowsError && flows && !items && (
                        <CodeEditor
                            value={JSON.stringify(flows, null, 2)}
                            editable={false}
                            title="flows.json"
                            className="mx-4 mb-4 min-h-0 [&_.code-pane]:min-h-[28vh] [&_.code-pane]:max-h-[40vh]"
                        />
                    )}
                </>
            )}
        </GlassCard>
    );
}

export default function Networking() {
    const [d, setD] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/network").then((r) => alive && setD(r.data));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!d) {
        return (
            <div data-testid={NET.root} className="page">
                <Skeleton className="h-40 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div data-testid={NET.root} className="page">
            <PageHeader
                title="Networking"
                description="Service graph across pods, services and endpoints."
                action={
                    <span className="mono" style={{ fontSize: 13, color: "var(--ink-3)" }}>
                        namespace: {d.namespace}
                    </span>
                }
            />

            <div className="grid g3">
                <div className="stat">
                    <Network size={18} style={{ color: "var(--blue)" }} />
                    <b style={{ marginTop: 12 }}>{d.nodes.length}</b>
                    <span>Graph nodes</span>
                </div>
                <div className="stat">
                    <ArrowRightLeft size={18} style={{ color: "var(--green)" }} />
                    <b style={{ marginTop: 12 }}>{d.edges.length}</b>
                    <span>Connections</span>
                </div>
                <div className="stat">
                    <Boxes size={18} style={{ color: "var(--purple)" }} />
                    <b style={{ marginTop: 12 }}>{d.nodes.filter((n) => n.kind === "service").length}</b>
                    <span>Services</span>
                </div>
            </div>

            <div className="grid g2" style={{ marginTop: 14 }}>
                <GlassCard className="overflow-hidden">
                    <div className="card-h">
                        <h2 style={{ fontSize: 15 }}>Graph nodes</h2>
                    </div>
                    <div style={{ overflowX: "auto" }} className="max-h-[520px] overflow-y-auto scrollbar-thin">
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>IP</th>
                                    <th>Ports</th>
                                </tr>
                            </thead>
                            <tbody>
                                {d.nodes.map((n) => (
                                    <tr key={n.id}>
                                        <td className="name">
                                            {n.label}
                                            <small>{n.kind} · {n.namespace}</small>
                                        </td>
                                        <td className="mono">{n.ip}</td>
                                        <td className="mono num">{(n.ports || []).join(", ")}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>

                <GlassCard className="overflow-hidden">
                    <div className="card-h">
                        <h2 style={{ fontSize: 15 }}>Connections</h2>
                        <span className="r" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12.5, color: "var(--ink-3)" }}>
                            <span className="status-dot ok animate-pulse" /> live
                        </span>
                    </div>
                    <div style={{ overflowX: "auto" }} className="max-h-[520px] overflow-y-auto scrollbar-thin">
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>From</th>
                                    <th>To</th>
                                    <th>Label</th>
                                </tr>
                            </thead>
                            <tbody>
                                {d.edges.map((e, i) => (
                                    <tr key={i}>
                                        <td className="mono">{e.from}</td>
                                        <td className="mono">{e.to}</td>
                                        <td className="num">{e.label}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>
            </div>

            <div style={{ marginTop: 14 }}>
                <PacketWolfPanel />
            </div>
            <div style={{ marginTop: 14 }}>
                <NetworkPolicyPanel />
            </div>
        </div>
    );
}
