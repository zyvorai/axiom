import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ShieldCheck, PlugZap, Save } from "lucide-react";
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

            <GlassCard className="p-5 text-sm text-muted-foreground">
                Postgres and probe integrations, and API-key sign-in, are configured via environment variables
                (<span className="mono text-foreground">AXIOM_*</span>) on the Axiom process. See the platform
                documentation for the full list.
            </GlassCard>
        </div>
    );
}
