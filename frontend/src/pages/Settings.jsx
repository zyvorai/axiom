import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ShieldCheck, PlugZap, Save, Database, Activity } from "lucide-react";
import { toast } from "sonner";

function Field({ label, ...props }) {
    return (
        <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{label}</Label>
            <Input className="border-white/10 bg-white/[0.03]" {...props} />
        </div>
    );
}

function IntegrationCard({ title, icon: Icon, configured, children, onSave, onTest, saving, testing }) {
    return (
        <GlassCard className="p-5">
            <div className="mb-4 flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <Icon className="h-4 w-4 text-[#ff7550]" />
                    <div className="text-sm font-medium">{title}</div>
                </div>
                <Badge
                    className={
                        configured
                            ? "border-[#93c88f]/40 bg-[#93c88f]/10 text-[#93c88f] hover:bg-[#93c88f]/10"
                            : "border-white/10 bg-white/[0.03] text-muted-foreground hover:bg-white/[0.03]"
                    }
                >
                    {configured ? "configured" : "not configured"}
                </Badge>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">{children}</div>
            <div className="mt-4 flex gap-2">
                <Button size="sm" onClick={onSave} disabled={saving} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                    <Save className="mr-1.5 h-3.5 w-3.5" /> {saving ? "Saving…" : "Save"}
                </Button>
                {onTest && (
                    <Button size="sm" variant="outline" onClick={onTest} disabled={testing} className="rounded-xl border-white/10 bg-white/[0.03]">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {testing ? "Testing…" : "Test connection"}
                    </Button>
                )}
            </div>
        </GlassCard>
    );
}

export default function Settings() {
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

    if (!data || !form) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Settings</h1>
                <p className="text-sm text-muted-foreground">Configure single sign-on and platform integrations.</p>
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

            <GlassCard className="p-5 text-sm text-muted-foreground">
                API-key sign-in is configured via the <span className="mono text-foreground">AXIOM_API_KEY</span>{" "}
                environment variable on the Axiom process.
            </GlassCard>
        </div>
    );
}
