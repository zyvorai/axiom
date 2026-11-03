import { useEffect, useId, useState } from "react";
import { useLocation } from "react-router-dom";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { ShieldCheck, PlugZap, Save, Database, Activity, BellRing, SunMoon, Cloud, Check } from "lucide-react";
import { toast } from "sonner";
import ThemeToggle from "@/components/ThemeToggle";
import { THEME_LABELS, useTheme } from "@/lib/theme";
import { useCluster } from "@/lib/cluster";
import StatusBadge from "@/components/StatusBadge";
import { NAV } from "@/constants/testIds";
import AiSettingsCard from "@/components/AiSettingsCard";

function Field({ label, id, ...props }) {
    const auto = useId();
    const fieldId = id || auto;
    return (
        <div>
            <Label htmlFor={fieldId} className="text-xs text-muted-foreground">{label}</Label>
            <Input id={fieldId} className="bg-muted/30 mt-1.5" {...props} />
        </div>
    );
}

function Pill({ on, onLabel, offLabel }) {
    return (
        <span className="pill" style={{ "--c": on ? "var(--green)" : "var(--ink-3)" }}>
            <i aria-hidden="true" />
            {on ? onLabel : offLabel}
        </span>
    );
}

function IntegrationCard({ title, icon: Icon, configured, children, onSave, onTest, saving, testing }) {
    return (
        <div className="card">
            <div className="form-section mb-0">
                <div className="flex items-start justify-between gap-3">
                    <h2 className="flex items-center gap-2">
                        <Icon size={16} />
                        {title}
                    </h2>
                    <Pill on={configured} onLabel="configured" offLabel="not configured" />
                </div>
                <div className="form-row mt-4">
                    {children}
                </div>
                <div className="flex flex-wrap gap-2.5">
                    <button type="button" className="btn btn-fill btn-sm" onClick={onSave} disabled={saving}>
                        <Save size={14} /> {saving ? "Saving…" : "Save"}
                    </button>
                    {onTest && (
                        <button type="button" className="btn btn-line btn-sm" onClick={onTest} disabled={testing}>
                            <PlugZap size={14} /> {testing ? "Testing…" : "Test connection"}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}

export default function Settings() {
    const { hash } = useLocation();
    const { preference } = useTheme();
    const { clusters, active, activate, loading: clustersLoading, refresh: refreshClusters } = useCluster();
    const [data, setData] = useState(null);
    const [form, setForm] = useState(null);
    const [busy, setBusy] = useState("");
    const [addClusterOpen, setAddClusterOpen] = useState(false);
    const [clusterForm, setClusterForm] = useState({ name: "", label: "", kubeconfig: "" });
    const [clusterBusy, setClusterBusy] = useState(false);

    useEffect(() => {
        let alive = true;
        api.get("/integrations").then((r) => {
            if (!alive) return;
            setData(r.data);
            setForm(r.data);
        });
        return () => {
            alive = false;
        };
    }, []);

    useEffect(() => {
        if (hash !== "#clusters") return;
        const t = requestAnimationFrame(() => {
            document.getElementById("clusters")?.scrollIntoView({ behavior: "smooth", block: "start" });
        });
        return () => cancelAnimationFrame(t);
    }, [hash, data]);

    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const saveOidc = async () => {
        setBusy("oidc-save");
        try {
            const { data: v } = await api.put("/integrations", {
                oidcIssuer: form.oidcIssuer,
                oidcLoginUrl: form.oidcLoginUrl,
                oidcClientId: form.oidcClientId,
            });
            setData(v);
            setForm(v);
            toast.success("OIDC settings saved");
        } catch (e) {
            toast.error("Save failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testOidc = async () => {
        setBusy("oidc-test");
        try {
            const { data: r } = await api.post("/integrations/oidc/test");
            r.ok ? toast.success("OIDC reachable", { description: r.hint }) : toast.error("OIDC test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const saveAtlas = async () => {
        setBusy("atlas-save");
        try {
            const { data: v } = await api.put("/integrations", {
                atlasUrl: form.atlasUrl,
                atlasConsoleUrl: form.atlasConsoleUrl,
            });
            setData(v);
            setForm(v);
            toast.success("Atlas settings saved");
        } catch (e) {
            toast.error("Save failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testAtlas = async () => {
        setBusy("atlas-test");
        try {
            const { data: r } = await api.post("/integrations/atlas/test");
            r.ok ? toast.success("Atlas reachable", { description: `${r.latencyMs}ms` }) : toast.error("Atlas test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const savePostgres = async () => {
        setBusy("postgres-save");
        try {
            const { data: v } = await api.put("/integrations", {
                postgresHost: form.postgresHost,
                postgresPort: Number(form.postgresPort) || 5432,
                postgresDatabase: form.postgresDatabase,
                postgresUser: form.postgresUser,
                postgresSslMode: form.postgresSslMode || "disable",
                ...(form.postgresPassword ? { postgresPassword: form.postgresPassword } : {}),
            });
            setData(v);
            setForm({ ...v, postgresPassword: "" });
            toast.success("Postgres settings saved");
        } catch (e) {
            toast.error("Save failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testPostgres = async () => {
        setBusy("postgres-test");
        try {
            const { data: r } = await api.post("/integrations/postgres/test");
            r.ok ? toast.success("Postgres reachable", { description: `${r.latencyMs}ms` }) : toast.error("Postgres test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const saveProbe = async () => {
        setBusy("probe-save");
        try {
            const { data: v } = await api.put("/integrations", {
                probePodNamespace: form.probePodNamespace,
                probePodName: form.probePodName,
                probePodContainer: form.probePodContainer,
                probeUrl: form.probeUrl,
            });
            setData(v);
            setForm(v);
            toast.success("Probe settings saved");
        } catch (e) {
            toast.error("Save failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testProbe = async () => {
        setBusy("probe-test");
        try {
            const { data: r } = await api.post("/integrations/probe/test");
            r.ok ? toast.success("Probe reachable", { description: `${r.latencyMs}ms` }) : toast.error("Probe test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const saveNotify = async () => {
        setBusy("notify-save");
        try {
            const { data: v } = await api.put("/integrations", {
                notifyOnWarnings: !!form.notifyOnWarnings,
                ...(form.slackWebhookUrl ? { slackWebhookUrl: form.slackWebhookUrl } : {}),
                ...(form.alertWebhookUrl ? { alertWebhookUrl: form.alertWebhookUrl } : {}),
            });
            setData(v);
            setForm({ ...v, slackWebhookUrl: "", alertWebhookUrl: "" });
            toast.success("Notification settings saved");
        } catch (e) {
            toast.error("Save failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testSlack = async () => {
        setBusy("slack-test");
        try {
            const { data: r } = await api.post("/integrations/slack/test");
            r.ok ? toast.success("Slack message sent") : toast.error("Slack test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testWebhook = async () => {
        setBusy("webhook-test");
        try {
            const { data: r } = await api.post("/integrations/webhook/test");
            r.ok ? toast.success("Webhook delivered") : toast.error("Webhook test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!data || !form) {
        return (
            <div className="page">
                <PageHeader title="Settings" />
                <div className="flex flex-col gap-3.5">
                    <Skeleton className="h-64 rounded-2xl bg-muted" />
                    <Skeleton className="h-64 rounded-2xl bg-muted" />
                </div>
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Settings"
                description="Clusters, appearance, single sign-on, and platform integrations."
            />

            <div className="stack">
                <div className="card scroll-mt-4" id="clusters">
                    <div className="form-section mb-0">
                        <div className="flex items-start justify-between gap-3 mb-1">
                            <h2 className="flex items-center gap-2">
                                <Cloud size={16} />
                                Clusters
                            </h2>
                            <div className="flex gap-2.5">
                                <button type="button" className="btn btn-line btn-sm" onClick={refreshClusters} disabled={clustersLoading}>
                                    Refresh
                                </button>
                                <button type="button" className="btn btn-fill btn-sm" onClick={() => setAddClusterOpen(true)}>
                                    Add cluster
                                </button>
                            </div>
                        </div>
                        <p className="lead">
                            Register remote kubeconfigs for fleet switching. Active cluster: <span className="mono">{active?.label || active?.name || "—"}</span>
                        </p>
                        {clustersLoading && !clusters.length ? (
                            <Skeleton className="h-24 rounded-xl bg-muted" />
                        ) : (clusters || []).length === 0 ? (
                            <div className="empty">No clusters configured.</div>
                        ) : (
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>Cluster</th>
                                        <th>Status</th>
                                        <th><span className="sr-only">Actions</span></th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {(clusters || []).map((c) => {
                                        const isActive = c.name === active?.name || c.current;
                                        return (
                                            <tr key={c.name} data-testid={NAV.clusterOption(c.name)}>
                                                <td className="name">
                                                    <span className="flex items-center gap-2">
                                                        <Check size={14} className={`shrink-0 text-tone-link ${isActive ? "opacity-100" : "opacity-0"}`} />
                                                        {c.label || c.name}
                                                        {isActive && <span className="pill soft">active</span>}
                                                    </span>
                                                    <small>{c.server || c.name}</small>
                                                </td>
                                                <td>
                                                    <span title={c.error || undefined}>
                                                        <StatusBadge status={c.state === "degraded" ? "Degraded" : c.state === "unknown" ? "Pending" : c.healthy ? "Healthy" : "NotReady"} />
                                                    </span>
                                                    {c.state && c.state !== "unknown" && <small>{c.state === "unreachable" ? "unreachable" : `${c.latencyMs} ms`}</small>}
                                                </td>
                                                <td>
                                                    {!isActive && (
                                                        <button type="button" className="btn btn-line btn-sm" onClick={() => activate(c.name)}>
                                                            Activate
                                                        </button>
                                                    )}
                                                </td>
                                            </tr>
                                        );
                                    })}
                                </tbody>
                            </table>
                        )}
                    </div>
                </div>

                <Dialog open={addClusterOpen} onOpenChange={setAddClusterOpen}>
                    <DialogContent className="max-w-lg">
                        <DialogHeader>
                            <DialogTitle>Register remote cluster</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-3">
                            <Field label="Name" value={clusterForm.name} onChange={(e) => setClusterForm((f) => ({ ...f, name: e.target.value }))} placeholder="prod-east" />
                            <Field label="Label" value={clusterForm.label} onChange={(e) => setClusterForm((f) => ({ ...f, label: e.target.value }))} placeholder="Production East" />
                            <div className="space-y-1.5">
                                <Label htmlFor="cluster-kubeconfig" className="text-xs text-muted-foreground">Kubeconfig YAML</Label>
                                <Textarea
                                    id="cluster-kubeconfig"
                                    className="mono min-h-[180px] bg-muted/30 text-xs"
                                    value={clusterForm.kubeconfig}
                                    onChange={(e) => setClusterForm((f) => ({ ...f, kubeconfig: e.target.value }))}
                                    placeholder={"apiVersion: v1\nkind: Config\n..."}
                                />
                            </div>
                        </div>
                        <DialogFooter>
                            <button
                                type="button"
                                className="btn btn-fill"
                                disabled={clusterBusy || !clusterForm.name || !clusterForm.kubeconfig}
                                onClick={async () => {
                                    setClusterBusy(true);
                                    try {
                                        await api.post("/clusters", clusterForm);
                                        toast.success(`Cluster ${clusterForm.name} registered`);
                                        setAddClusterOpen(false);
                                        setClusterForm({ name: "", label: "", kubeconfig: "" });
                                        refreshClusters();
                                    } catch (e) {
                                        toast.error("Register failed", { description: e?.response?.data?.error || e.message });
                                    } finally {
                                        setClusterBusy(false);
                                    }
                                }}
                            >
                                {clusterBusy ? "Registering…" : "Register"}
                            </button>
                        </DialogFooter>
                    </DialogContent>
                </Dialog>

                <div className="card">
                    <div className="form-section mb-0">
                        <h2 className="flex items-center gap-2">
                            <SunMoon size={16} />
                            Appearance
                        </h2>
                        <p className="lead">Theme is stored in this browser. System follows your device appearance.</p>
                        <div className="rule-row">
                            <div>
                                <div className="text-[14px]">Theme</div>
                                <div className="text-[12.5px] text-ink-3">
                                    {THEME_LABELS[preference] || "Dark"}
                                </div>
                            </div>
                            <ThemeToggle />
                        </div>
                    </div>
                </div>

                <IntegrationCard
                    title="OIDC single sign-on"
                    icon={ShieldCheck}
                    configured={data.oidcConfigured}
                    onSave={saveOidc}
                    onTest={testOidc}
                    saving={busy === "oidc-save"}
                    testing={busy === "oidc-test"}
                >
                    <Field label="Issuer" value={form.oidcIssuer || ""} onChange={(e) => set({ oidcIssuer: e.target.value })} placeholder="https://issuer.example.com" />
                    <Field label="Login URL" value={form.oidcLoginUrl || ""} onChange={(e) => set({ oidcLoginUrl: e.target.value })} placeholder="https://issuer.example.com/authorize" />
                    <Field label="Client ID" value={form.oidcClientId || ""} onChange={(e) => set({ oidcClientId: e.target.value })} />
                </IntegrationCard>

                <IntegrationCard
                    title="Atlas"
                    icon={PlugZap}
                    configured={data.atlasConfigured}
                    onSave={saveAtlas}
                    onTest={testAtlas}
                    saving={busy === "atlas-save"}
                    testing={busy === "atlas-test"}
                >
                    <Field label="Atlas URL" value={form.atlasUrl || ""} onChange={(e) => set({ atlasUrl: e.target.value })} placeholder="https://atlas.internal" />
                    <Field label="Console URL" value={form.atlasConsoleUrl || ""} onChange={(e) => set({ atlasConsoleUrl: e.target.value })} />
                </IntegrationCard>

                <IntegrationCard
                    title="Postgres"
                    icon={Database}
                    configured={data.postgresConfigured}
                    onSave={savePostgres}
                    onTest={testPostgres}
                    saving={busy === "postgres-save"}
                    testing={busy === "postgres-test"}
                >
                    <Field label="Host" value={form.postgresHost || ""} onChange={(e) => set({ postgresHost: e.target.value })} placeholder="postgres.internal" />
                    <Field label="Port" type="number" value={form.postgresPort || ""} onChange={(e) => set({ postgresPort: e.target.value })} placeholder="5432" />
                    <Field label="Database" value={form.postgresDatabase || ""} onChange={(e) => set({ postgresDatabase: e.target.value })} />
                    <Field label="User" value={form.postgresUser || ""} onChange={(e) => set({ postgresUser: e.target.value })} />
                    <Field
                        label={data.postgresPasswordSet ? "Password (set — leave blank to keep)" : "Password"}
                        type="password"
                        value={form.postgresPassword || ""}
                        onChange={(e) => set({ postgresPassword: e.target.value })}
                    />
                    <Field label="SSL mode" value={form.postgresSslMode || "disable"} onChange={(e) => set({ postgresSslMode: e.target.value })} placeholder="disable" />
                </IntegrationCard>

                <IntegrationCard
                    title="Probe"
                    icon={Activity}
                    configured={data.probeConfigured}
                    onSave={saveProbe}
                    onTest={testProbe}
                    saving={busy === "probe-save"}
                    testing={busy === "probe-test"}
                >
                    <Field label="Pod namespace" value={form.probePodNamespace || ""} onChange={(e) => set({ probePodNamespace: e.target.value })} placeholder="default" />
                    <Field label="Pod name" value={form.probePodName || ""} onChange={(e) => set({ probePodName: e.target.value })} />
                    <Field label="Container" value={form.probePodContainer || ""} onChange={(e) => set({ probePodContainer: e.target.value })} />
                    <Field label="Probe URL" value={form.probeUrl || ""} onChange={(e) => set({ probeUrl: e.target.value })} placeholder="http://localhost:8080/healthz" />
                </IntegrationCard>

                <div className="card">
                    <div className="form-section mb-0">
                        <div className="flex items-start justify-between gap-3">
                            <h2 className="flex items-center gap-2">
                                <BellRing size={16} />
                                Notifications
                            </h2>
                            <Pill on={data.notifyConfigured} onLabel="configured" offLabel="not configured" />
                        </div>
                        <div className="form-row mt-4">
                            <Field
                                label={data.slackWebhookUrlSet ? "Slack webhook URL (set — leave blank to keep)" : "Slack webhook URL"}
                                type="password"
                                value={form.slackWebhookUrl || ""}
                                onChange={(e) => set({ slackWebhookUrl: e.target.value })}
                                placeholder="https://hooks.slack.com/services/…"
                            />
                            <Field
                                label={data.alertWebhookUrlSet ? "Alert webhook URL (set — leave blank to keep)" : "Alert webhook URL"}
                                type="password"
                                value={form.alertWebhookUrl || ""}
                                onChange={(e) => set({ alertWebhookUrl: e.target.value })}
                                placeholder="https://your-endpoint.example.com/hooks/axiom"
                            />
                        </div>
                        <div className="rule-row mb-4">
                            <div>
                                <div className="text-[14px]">Forward Warning events automatically</div>
                                <div className="text-[12.5px] text-ink-3">Polls every 60s and pushes new cluster Warning events to the channels above.</div>
                            </div>
                            <Switch aria-label="Forward Warning events automatically" checked={!!form.notifyOnWarnings} onCheckedChange={(v) => set({ notifyOnWarnings: v })} />
                        </div>
                        <div className="flex flex-wrap gap-2.5">
                            <button type="button" className="btn btn-fill btn-sm" onClick={saveNotify} disabled={busy === "notify-save"}>
                                <Save size={14} /> {busy === "notify-save" ? "Saving…" : "Save"}
                            </button>
                            <button type="button" className="btn btn-line btn-sm" onClick={testSlack} disabled={busy === "slack-test"}>
                                <PlugZap size={14} /> {busy === "slack-test" ? "Testing…" : "Test Slack"}
                            </button>
                            <button type="button" className="btn btn-line btn-sm" onClick={testWebhook} disabled={busy === "webhook-test"}>
                                <PlugZap size={14} /> {busy === "webhook-test" ? "Testing…" : "Test webhook"}
                            </button>
                        </div>
                    </div>
                </div>

                <AiSettingsCard data={data} onSaved={setData} />

                <div className="card text-[13.5px] text-ink-2">
                    API-key sign-in is configured via the <span className="mono">AXIOM_API_KEY</span> environment variable on the Axiom process.
                </div>
            </div>
        </div>
    );
}
