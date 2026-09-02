import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { ShieldCheck, PlugZap, Save, Database, Activity, BellRing, SunMoon, Cloud, Check } from "lucide-react";
import { toast } from "sonner";
import ThemeToggle from "@/components/ThemeToggle";
import { useTheme } from "@/lib/theme";
import { useCluster } from "@/lib/cluster";
import StatusBadge from "@/components/StatusBadge";
import { NAV } from "@/constants/testIds";

function Field({ label, ...props }) {
    return (
        <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{label}</Label>
            <Input className="bg-muted/30" {...props} />
        </div>
    );
}

function IntegrationCard({ title, icon: Icon, configured, children, onSave, onTest, saving, testing }) {
    return (
        <GlassCard className="p-5">
            <div className="mb-4 flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <Icon className="h-4 w-4 text-i17-cosmic" />
                    <div className="text-sm font-medium">{title}</div>
                </div>
                <Badge
                    className={
                        configured
                            ? "border-i17-sage/40 bg-i17-sage/10 text-i17-sage hover:bg-i17-sage/10"
                            : "bg-muted/30 text-muted-foreground hover:bg-muted"
                    }
                >
                    {configured ? "configured" : "not configured"}
                </Badge>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">{children}</div>
            <div className="mt-4 flex gap-2">
                <Button size="sm" onClick={onSave} disabled={saving} className="rounded-xl">
                    <Save className="mr-1.5 h-3.5 w-3.5" /> {saving ? "Saving…" : "Save"}
                </Button>
                {onTest && (
                    <Button size="sm" variant="outline" onClick={onTest} disabled={testing} className="rounded-xl bg-muted/30">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {testing ? "Testing…" : "Test connection"}
                    </Button>
                )}
            </div>
        </GlassCard>
    );
}

export default function Settings() {
    const { hash } = useLocation();
    const { theme } = useTheme();
    const { clusters, active, activate, loading: clustersLoading, refresh: refreshClusters } = useCluster();
    const [data, setData] = useState(null);
    const [form, setForm] = useState(null);
    const [busy, setBusy] = useState("");

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
        } finally {
            setBusy("");
        }
    };

    const testWebhook = async () => {
        setBusy("webhook-test");
        try {
            const { data: r } = await api.post("/integrations/webhook/test");
            r.ok ? toast.success("Webhook delivered") : toast.error("Webhook test failed", { description: r.error || r.hint });
        } finally {
            setBusy("");
        }
    };

    if (!data || !form) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-64 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <PageHeader
                eyebrow="SETTINGS"
                title="Settings"
                description="Clusters, appearance, single sign-on, and platform integrations."
            />

            <GlassCard id="clusters" className="scroll-mt-4 p-5">
                <div className="mb-4 flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                        <Cloud className="h-4 w-4 text-i17-cosmic" />
                        <div className="text-sm font-medium">Clusters</div>
                    </div>
                    <Button size="sm" variant="outline" onClick={refreshClusters} disabled={clustersLoading} className="rounded-xl">
                        Refresh
                    </Button>
                </div>
                <p className="mb-3 text-xs text-muted-foreground">
                    Health and activation for kubeconfigs wired into Axiom. Active cluster:{" "}
                    <span className="mono text-foreground">{active?.label || active?.name || "—"}</span>
                </p>
                {clustersLoading && !clusters.length ? (
                    <Skeleton className="h-24 rounded-xl bg-muted" />
                ) : (
                    <div className="divide-y border-y border-border">
                        {(clusters || []).map((c) => {
                            const isActive = c.name === active?.name || c.current;
                            return (
                                <div
                                    key={c.name}
                                    data-testid={NAV.clusterOption(c.name)}
                                    className="flex items-center gap-3 px-3 py-3"
                                >
                                    <Check className={`h-4 w-4 shrink-0 ${isActive ? "opacity-100 text-primary" : "opacity-0"}`} />
                                    <div className="min-w-0 flex-1">
                                        <div className="flex flex-wrap items-center gap-2">
                                            <span className="truncate text-sm font-medium">{c.label || c.name}</span>
                                            <StatusBadge status={c.healthy ? "Healthy" : "NotReady"} />
                                            {isActive && (
                                                <Badge variant="secondary" className="text-[10px]">
                                                    active
                                                </Badge>
                                            )}
                                        </div>
                                        <div className="mt-0.5 truncate text-xs text-muted-foreground">{c.server || c.name}</div>
                                    </div>
                                    {!isActive && (
                                        <Button size="sm" variant="outline" onClick={() => activate(c.name)} className="rounded-xl shrink-0">
                                            Activate
                                        </Button>
                                    )}
                                </div>
                            );
                        })}
                        {(clusters || []).length === 0 && (
                            <div className="px-3 py-8 text-center text-sm text-muted-foreground">No clusters configured.</div>
                        )}
                    </div>
                )}
            </GlassCard>

            <GlassCard className="p-5">
                <div className="mb-4 flex items-center gap-2">
                    <SunMoon className="h-4 w-4 text-i17-cosmic" />
                    <div className="text-sm font-medium">Appearance</div>
                </div>
                <div className="flex items-center justify-between gap-4 border-y border-border px-3 py-2.5">
                    <div>
                        <div className="text-sm">Theme</div>
                        <div className="text-xs text-muted-foreground">
                            {theme === "dark" ? "Dark (GitHub Primer)" : "Light (Apple)"} — stored in this browser.
                        </div>
                    </div>
                    <ThemeToggle />
                </div>
            </GlassCard>

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

            <GlassCard className="p-5">
                <div className="mb-4 flex items-center justify-between">
                    <div className="flex items-center gap-2">
                        <BellRing className="h-4 w-4 text-i17-cosmic" />
                        <div className="text-sm font-medium">Notifications</div>
                    </div>
                    <Badge
                        className={
                            data.notifyConfigured
                                ? "border-i17-sage/40 bg-i17-sage/10 text-i17-sage hover:bg-i17-sage/10"
                                : "bg-muted/30 text-muted-foreground hover:bg-muted"
                        }
                    >
                        {data.notifyConfigured ? "configured" : "not configured"}
                    </Badge>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
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
                <div className="mt-3 flex items-center justify-between border-y border-border px-3 py-2.5">
                    <div>
                        <div className="text-sm">Forward Warning events automatically</div>
                        <div className="text-xs text-muted-foreground">Polls every 60s and pushes new cluster Warning events to the channels above.</div>
                    </div>
                    <Switch checked={!!form.notifyOnWarnings} onCheckedChange={(v) => set({ notifyOnWarnings: v })} />
                </div>
                <div className="mt-4 flex flex-wrap gap-2">
                    <Button size="sm" onClick={saveNotify} disabled={busy === "notify-save"} className="rounded-xl">
                        <Save className="mr-1.5 h-3.5 w-3.5" /> {busy === "notify-save" ? "Saving…" : "Save"}
                    </Button>
                    <Button size="sm" variant="outline" onClick={testSlack} disabled={busy === "slack-test"} className="rounded-xl bg-muted/30">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {busy === "slack-test" ? "Testing…" : "Test Slack"}
                    </Button>
                    <Button size="sm" variant="outline" onClick={testWebhook} disabled={busy === "webhook-test"} className="rounded-xl bg-muted/30">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {busy === "webhook-test" ? "Testing…" : "Test webhook"}
                    </Button>
                </div>
            </GlassCard>

            <GlassCard className="p-5 text-sm text-muted-foreground">
                API-key sign-in is configured via the <span className="mono text-foreground">AXIOM_API_KEY</span>{" "}
                environment variable on the Axiom process.
            </GlassCard>
        </div>
    );
}
