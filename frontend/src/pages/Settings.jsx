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
import { useTranslation, Trans } from "react-i18next";
import ThemeToggle from "@/components/ThemeToggle";
import { useTheme } from "@/lib/theme";
import { useCluster } from "@/lib/cluster";
import StatusBadge from "@/components/StatusBadge";
import { NAV } from "@/constants/testIds";
import AiSettingsCard from "@/components/AiSettingsCard";
import BillingSettingsCard from "@/components/BillingSettingsCard";
import LanguageSelect from "@/components/LanguageSelect";

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
    const { t } = useTranslation();
    return (
        <div className="card">
            <div className="form-section mb-0">
                <div className="flex items-start justify-between gap-3">
                    <h2 className="flex items-center gap-2">
                        <Icon size={16} />
                        {title}
                    </h2>
                    <Pill on={configured} onLabel={t("pages:settings.configured")} offLabel={t("pages:settings.notConfigured")} />
                </div>
                <div className="form-row mt-4">
                    {children}
                </div>
                <div className="flex flex-wrap gap-2.5">
                    <button type="button" className="btn btn-fill btn-sm" onClick={onSave} disabled={saving}>
                        <Save size={14} /> {saving ? t("common:actions.saving") : t("common:actions.save")}
                    </button>
                    {onTest && (
                        <button type="button" className="btn btn-line btn-sm" onClick={onTest} disabled={testing}>
                            <PlugZap size={14} /> {testing ? t("pages:settings.testing") : t("pages:settings.testConnection")}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}

export default function Settings() {
    const { t } = useTranslation();
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
        const raf = requestAnimationFrame(() => {
            document.getElementById("clusters")?.scrollIntoView({ behavior: "smooth", block: "start" });
        });
        return () => cancelAnimationFrame(raf);
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
            toast.success(t("toasts:settings.oidcSaved"));
        } catch (e) {
            toast.error(t("toasts:settings.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testOidc = async () => {
        setBusy("oidc-test");
        try {
            const { data: r } = await api.post("/integrations/oidc/test");
            r.ok ? toast.success(t("toasts:settings.oidcReachable"), { description: r.hint }) : toast.error(t("toasts:settings.oidcTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:settings.testFailed"), { description: e?.response?.data?.error || e.message });
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
            toast.success(t("toasts:settings.atlasSaved"));
        } catch (e) {
            toast.error(t("toasts:settings.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testAtlas = async () => {
        setBusy("atlas-test");
        try {
            const { data: r } = await api.post("/integrations/atlas/test");
            r.ok ? toast.success(t("toasts:settings.atlasReachable"), { description: `${r.latencyMs}ms` }) : toast.error(t("toasts:settings.atlasTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:settings.testFailed"), { description: e?.response?.data?.error || e.message });
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
            toast.success(t("toasts:settings.postgresSaved"));
        } catch (e) {
            toast.error(t("toasts:settings.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testPostgres = async () => {
        setBusy("postgres-test");
        try {
            const { data: r } = await api.post("/integrations/postgres/test");
            r.ok ? toast.success(t("toasts:settings.postgresReachable"), { description: `${r.latencyMs}ms` }) : toast.error(t("toasts:settings.postgresTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:settings.testFailed"), { description: e?.response?.data?.error || e.message });
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
            toast.success(t("toasts:settings.probeSaved"));
        } catch (e) {
            toast.error(t("toasts:settings.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testProbe = async () => {
        setBusy("probe-test");
        try {
            const { data: r } = await api.post("/integrations/probe/test");
            r.ok ? toast.success(t("toasts:settings.probeReachable"), { description: `${r.latencyMs}ms` }) : toast.error(t("toasts:settings.probeTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:settings.testFailed"), { description: e?.response?.data?.error || e.message });
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
            toast.success(t("toasts:settings.notifySaved"));
        } catch (e) {
            toast.error(t("toasts:settings.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testSlack = async () => {
        setBusy("slack-test");
        try {
            const { data: r } = await api.post("/integrations/slack/test");
            r.ok ? toast.success(t("toasts:settings.slackSent")) : toast.error(t("toasts:settings.slackTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:settings.testFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const testWebhook = async () => {
        setBusy("webhook-test");
        try {
            const { data: r } = await api.post("/integrations/webhook/test");
            r.ok ? toast.success(t("toasts:settings.webhookDelivered")) : toast.error(t("toasts:settings.webhookTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:settings.testFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!data || !form) {
        return (
            <div className="page">
                <PageHeader title={t("pages:settings.title")} />
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
                title={t("pages:settings.title")}
                description={t("pages:settings.description")}
            />

            <div className="stack">
                <div className="card scroll-mt-4" id="clusters">
                    <div className="form-section mb-0">
                        <div className="flex items-start justify-between gap-3 mb-1">
                            <h2 className="flex items-center gap-2">
                                <Cloud size={16} />
                                {t("pages:settings.clusters.title")}
                            </h2>
                            <div className="flex gap-2.5">
                                <button type="button" className="btn btn-line btn-sm" onClick={refreshClusters} disabled={clustersLoading}>
                                    {t("common:actions.refresh")}
                                </button>
                                <button type="button" className="btn btn-fill btn-sm" onClick={() => setAddClusterOpen(true)}>
                                    {t("pages:settings.clusters.add")}
                                </button>
                            </div>
                        </div>
                        <p className="lead">
                            <Trans
                                i18nKey="pages:settings.clusters.lead"
                                values={{ name: active?.label || active?.name || "—" }}
                                components={{ mono: <span className="mono" /> }}
                            />
                        </p>
                        {clustersLoading && !clusters.length ? (
                            <Skeleton className="h-24 rounded-xl bg-muted" />
                        ) : (clusters || []).length === 0 ? (
                            <div className="empty">{t("pages:settings.clusters.empty")}</div>
                        ) : (
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>{t("pages:settings.clusters.columns.cluster")}</th>
                                        <th>{t("common:fields.status")}</th>
                                        <th><span className="sr-only">{t("common:fields.actions")}</span></th>
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
                                                        {isActive && <span className="pill soft">{t("pages:settings.clusters.active")}</span>}
                                                    </span>
                                                    <small>{c.server || c.name}</small>
                                                </td>
                                                <td>
                                                    <span title={c.error || undefined}>
                                                        <StatusBadge status={c.state === "degraded" ? "Degraded" : c.state === "unknown" ? "Pending" : c.healthy ? "Healthy" : "NotReady"} />
                                                    </span>
                                                    {c.state && c.state !== "unknown" && <small>{c.state === "unreachable" ? t("pages:settings.clusters.unreachable") : t("pages:settings.clusters.latency", { ms: c.latencyMs })}</small>}
                                                </td>
                                                <td>
                                                    {!isActive && (
                                                        <button type="button" className="btn btn-line btn-sm" onClick={() => activate(c.name)}>
                                                            {t("pages:settings.clusters.activate")}
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
                            <DialogTitle>{t("dialogs:settings.registerCluster.title")}</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-3">
                            <Field label={t("common:fields.name")} value={clusterForm.name} onChange={(e) => setClusterForm((f) => ({ ...f, name: e.target.value }))} placeholder="prod-east" />
                            <Field label={t("dialogs:settings.registerCluster.label")} value={clusterForm.label} onChange={(e) => setClusterForm((f) => ({ ...f, label: e.target.value }))} placeholder="Production East" />
                            <div className="space-y-1.5">
                                <Label htmlFor="cluster-kubeconfig" className="text-xs text-muted-foreground">{t("dialogs:settings.registerCluster.kubeconfig")}</Label>
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
                                        toast.success(t("toasts:settings.clusterRegistered", { name: clusterForm.name }));
                                        setAddClusterOpen(false);
                                        setClusterForm({ name: "", label: "", kubeconfig: "" });
                                        refreshClusters();
                                    } catch (e) {
                                        toast.error(t("toasts:settings.registerFailed"), { description: e?.response?.data?.error || e.message });
                                    } finally {
                                        setClusterBusy(false);
                                    }
                                }}
                            >
                                {clusterBusy ? t("dialogs:settings.registerCluster.registering") : t("dialogs:settings.registerCluster.register")}
                            </button>
                        </DialogFooter>
                    </DialogContent>
                </Dialog>

                <div className="card">
                    <div className="form-section mb-0">
                        <h2 className="flex items-center gap-2">
                            <SunMoon size={16} />
                            {t("pages:settings.appearance.title")}
                        </h2>
                        <p className="lead">{t("pages:settings.appearance.lead")}</p>
                        <div className="rule-row">
                            <div>
                                <div className="text-[14px]">{t("pages:settings.appearance.theme")}</div>
                                <div className="text-[12.5px] text-ink-3">
                                    {{
                                        light: t("pages:settings.appearance.light"),
                                        dark: t("pages:settings.appearance.dark"),
                                        system: t("pages:settings.appearance.system"),
                                    }[preference] || t("pages:settings.appearance.dark")}
                                </div>
                            </div>
                            <ThemeToggle />
                        </div>
                        <div className="rule-row">
                            <div>
                                <div className="text-[14px]">{t("common:language.label")}</div>
                                <div className="text-[12.5px] text-ink-3">{t("common:language.hint")}</div>
                            </div>
                            <LanguageSelect compact />
                        </div>
                    </div>
                </div>

                <IntegrationCard
                    title={t("pages:settings.oidc.title")}
                    icon={ShieldCheck}
                    configured={data.oidcConfigured}
                    onSave={saveOidc}
                    onTest={testOidc}
                    saving={busy === "oidc-save"}
                    testing={busy === "oidc-test"}
                >
                    <Field label={t("pages:settings.oidc.issuer")} value={form.oidcIssuer || ""} onChange={(e) => set({ oidcIssuer: e.target.value })} placeholder="https://issuer.example.com" />
                    <Field label={t("pages:settings.oidc.loginUrl")} value={form.oidcLoginUrl || ""} onChange={(e) => set({ oidcLoginUrl: e.target.value })} placeholder="https://issuer.example.com/authorize" />
                    <Field label={t("pages:settings.oidc.clientId")} value={form.oidcClientId || ""} onChange={(e) => set({ oidcClientId: e.target.value })} />
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
                    <Field label={t("pages:settings.atlas.url")} value={form.atlasUrl || ""} onChange={(e) => set({ atlasUrl: e.target.value })} placeholder="https://atlas.internal" />
                    <Field label={t("pages:settings.atlas.consoleUrl")} value={form.atlasConsoleUrl || ""} onChange={(e) => set({ atlasConsoleUrl: e.target.value })} />
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
                    <Field label={t("pages:settings.postgres.host")} value={form.postgresHost || ""} onChange={(e) => set({ postgresHost: e.target.value })} placeholder="postgres.internal" />
                    <Field label={t("pages:settings.postgres.port")} type="number" value={form.postgresPort || ""} onChange={(e) => set({ postgresPort: e.target.value })} placeholder="5432" />
                    <Field label={t("pages:settings.postgres.database")} value={form.postgresDatabase || ""} onChange={(e) => set({ postgresDatabase: e.target.value })} />
                    <Field label={t("pages:settings.postgres.user")} value={form.postgresUser || ""} onChange={(e) => set({ postgresUser: e.target.value })} />
                    <Field
                        label={data.postgresPasswordSet ? t("pages:settings.postgres.passwordSet") : t("pages:settings.postgres.password")}
                        type="password"
                        value={form.postgresPassword || ""}
                        onChange={(e) => set({ postgresPassword: e.target.value })}
                    />
                    <Field label={t("pages:settings.postgres.sslMode")} value={form.postgresSslMode || "disable"} onChange={(e) => set({ postgresSslMode: e.target.value })} placeholder="disable" />
                </IntegrationCard>

                <IntegrationCard
                    title={t("pages:settings.probe.title")}
                    icon={Activity}
                    configured={data.probeConfigured}
                    onSave={saveProbe}
                    onTest={testProbe}
                    saving={busy === "probe-save"}
                    testing={busy === "probe-test"}
                >
                    <Field label={t("pages:settings.probe.podNamespace")} value={form.probePodNamespace || ""} onChange={(e) => set({ probePodNamespace: e.target.value })} placeholder="default" />
                    <Field label={t("pages:settings.probe.podName")} value={form.probePodName || ""} onChange={(e) => set({ probePodName: e.target.value })} />
                    <Field label={t("pages:settings.probe.container")} value={form.probePodContainer || ""} onChange={(e) => set({ probePodContainer: e.target.value })} />
                    <Field label={t("pages:settings.probe.url")} value={form.probeUrl || ""} onChange={(e) => set({ probeUrl: e.target.value })} placeholder="http://localhost:8080/healthz" />
                </IntegrationCard>

                <div className="card">
                    <div className="form-section mb-0">
                        <div className="flex items-start justify-between gap-3">
                            <h2 className="flex items-center gap-2">
                                <BellRing size={16} />
                                {t("pages:settings.notifications.title")}
                            </h2>
                            <Pill on={data.notifyConfigured} onLabel={t("pages:settings.configured")} offLabel={t("pages:settings.notConfigured")} />
                        </div>
                        <div className="form-row mt-4">
                            <Field
                                label={data.slackWebhookUrlSet ? t("pages:settings.notifications.slackWebhookSet") : t("pages:settings.notifications.slackWebhook")}
                                type="password"
                                value={form.slackWebhookUrl || ""}
                                onChange={(e) => set({ slackWebhookUrl: e.target.value })}
                                placeholder="https://hooks.slack.com/services/…"
                            />
                            <Field
                                label={data.alertWebhookUrlSet ? t("pages:settings.notifications.alertWebhookSet") : t("pages:settings.notifications.alertWebhook")}
                                type="password"
                                value={form.alertWebhookUrl || ""}
                                onChange={(e) => set({ alertWebhookUrl: e.target.value })}
                                placeholder="https://your-endpoint.example.com/hooks/axiom"
                            />
                        </div>
                        <div className="rule-row mb-4">
                            <div>
                                <div className="text-[14px]">{t("pages:settings.notifications.forwardWarnings")}</div>
                                <div className="text-[12.5px] text-ink-3">{t("pages:settings.notifications.forwardWarningsHint")}</div>
                            </div>
                            <Switch aria-label={t("pages:settings.notifications.forwardWarnings")} checked={!!form.notifyOnWarnings} onCheckedChange={(v) => set({ notifyOnWarnings: v })} />
                        </div>
                        <div className="flex flex-wrap gap-2.5">
                            <button type="button" className="btn btn-fill btn-sm" onClick={saveNotify} disabled={busy === "notify-save"}>
                                <Save size={14} /> {busy === "notify-save" ? t("common:actions.saving") : t("common:actions.save")}
                            </button>
                            <button type="button" className="btn btn-line btn-sm" onClick={testSlack} disabled={busy === "slack-test"}>
                                <PlugZap size={14} /> {busy === "slack-test" ? t("pages:settings.testing") : t("pages:settings.notifications.testSlack")}
                            </button>
                            <button type="button" className="btn btn-line btn-sm" onClick={testWebhook} disabled={busy === "webhook-test"}>
                                <PlugZap size={14} /> {busy === "webhook-test" ? t("pages:settings.testing") : t("pages:settings.notifications.testWebhook")}
                            </button>
                        </div>
                    </div>
                </div>

                <AiSettingsCard data={data} onSaved={setData} />

                <BillingSettingsCard data={data} onSaved={setData} />

                <div className="card text-[13.5px] text-ink-2">
                    <Trans i18nKey="pages:settings.apiKeyNote" components={{ mono: <span className="mono" /> }} />
                </div>
            </div>
        </div>
    );
}
