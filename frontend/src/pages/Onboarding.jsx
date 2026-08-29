import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
    ShieldCheck,
    Cloudy,
    Radar,
    KeyRound,
    Database,
    Activity,
    CheckCircle2,
    Circle,
    ArrowRight,
} from "lucide-react";

function useSetupStatus() {
    const [data, setData] = useState(null);
    useEffect(() => {
        let alive = true;
        Promise.all([
            api.get("/integrations").catch(() => ({ data: {} })),
            api.get("/packetwolf/status").catch(() => ({ data: {} })),
            api.get("/auth/providers").catch(() => ({ data: {} })),
            api.get("/session").catch(() => ({ data: {} })),
        ]).then(([integrations, packetwolf, providers, session]) => {
            if (!alive) return;
            setData({ integrations: integrations.data, packetwolf: packetwolf.data, providers: providers.data, session: session.data });
        });
        return () => {
            alive = false;
        };
    }, []);
    return data;
}

function Step({ done, required, icon: Icon, title, description, to, cta }) {
    return (
        <div className="flex items-start gap-4 border-b border-white/5 px-5 py-4 last:border-none">
            <div className="mt-0.5 shrink-0">
                {done ? <CheckCircle2 className="h-5 w-5 text-emerald-400" /> : <Circle className="h-5 w-5 text-muted-foreground" />}
            </div>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-white/[0.04]">
                <Icon className="h-4 w-4 text-i17-cosmic" />
            </div>
            <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                    <div className="text-sm font-medium">{title}</div>
                    {!required && (
                        <Badge variant="outline" className="border-white/10 bg-white/[0.03] text-[10px] text-muted-foreground">
                            optional
                        </Badge>
                    )}
                    {done && (
                        <Badge className="border-emerald-500/40 bg-emerald-500/10 text-[10px] text-emerald-300 hover:bg-emerald-500/10">
                            done
                        </Badge>
                    )}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
            </div>
            {!done && to && (
                <Link
                    to={to}
                    className="mt-0.5 flex shrink-0 items-center gap-1 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5 text-xs font-medium hover:bg-white/[0.08]"
                >
                    {cta || "Configure"} <ArrowRight className="h-3 w-3" />
                </Link>
            )}
        </div>
    );
}

export default function Onboarding() {
    const status = useSetupStatus();
    const { user } = useAuth();

    if (!status) return <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />;

    const { integrations, packetwolf, providers, session } = status;
    const ssoConfigured = !!(providers?.oidc?.enabled || providers?.saml?.enabled);
    const usingDemoDefaults = session?.mode === "demo" || (providers?.lab?.operator_login && providers?.local?.default_username === "demo");

    const steps = [
        {
            done: ssoConfigured,
            required: true,
            icon: ShieldCheck,
            title: "Connect single sign-on",
            description: ssoConfigured
                ? "OIDC or SAML is configured — the SSO buttons on the login screen are live."
                : "No OIDC/SAML provider configured yet. Local username/password sign-in works, but SSO is off.",
            to: "/settings",
        },
        {
            done: !usingDemoDefaults,
            required: true,
            icon: KeyRound,
            title: "Move off the demo/demo default credential",
            description: usingDemoDefaults
                ? "This instance is running with lab defaults (AXIOM_MODE=demo or the demo/demo local login). Set AXIOM_USERNAME/AXIOM_PASSWORD (or SSO) before this leaves the lab."
                : "Not running with lab default credentials.",
            to: null,
        },
        {
            done: !!integrations?.atlasConfigured,
            required: false,
            icon: Cloudy,
            title: "Connect Atlas storage",
            description: integrations?.atlasConfigured
                ? "Atlas is wired — the Atlas page shows live backends, pools and volumes."
                : "No storage control plane connected. The Atlas page will show volumes, pools and let you provision storage once configured.",
            to: "/atlas",
            cta: "View Atlas",
        },
        {
            done: !!packetwolf?.configured,
            required: false,
            icon: Radar,
            title: "Connect PacketWolf networking",
            description: packetwolf?.configured
                ? packetwolf.reachable
                    ? "PacketWolf is connected — live eBPF flow data shows on the Networking page."
                    : "PacketWolf URL is set but unreachable — check AXIOM_PACKETWOLF_URL."
                : "Set AXIOM_PACKETWOLF_URL to see live Cilium/eBPF flow data on the Networking page.",
            to: "/networking",
            cta: "View Networking",
        },
        {
            done: !!integrations?.postgresConfigured,
            required: false,
            icon: Database,
            title: "Connect Postgres",
            description: "Only needed if you use Postgres-backed features. Configure host/credentials in Settings.",
            to: "/settings",
        },
        {
            done: !!integrations?.probeConfigured,
            required: false,
            icon: Activity,
            title: "Connect an external probe",
            description: "Optional synthetic-check target for platform health. Configure in Settings.",
            to: "/settings",
        },
    ];

    const requiredSteps = steps.filter((s) => s.required);
    const doneCount = steps.filter((s) => s.done).length;
    const requiredDone = requiredSteps.filter((s) => s.done).length;

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Getting Started</h1>
                <p className="text-sm text-muted-foreground">
                    {user ? `Welcome, ${user.name}. ` : ""}What's configured on this Axiom instance, and what's left to wire up.
                </p>
            </div>

            <GlassCard className="p-5">
                <div className="flex items-center justify-between">
                    <div>
                        <div className="text-sm font-medium">Setup progress</div>
                        <div className="mt-0.5 text-xs text-muted-foreground">
                            {requiredDone}/{requiredSteps.length} required · {doneCount}/{steps.length} total
                        </div>
                    </div>
                    <div className="text-2xl font-semibold tabular-nums">{Math.round((doneCount / steps.length) * 100)}%</div>
                </div>
                <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-white/[0.06]">
                    <div
                        className="h-full rounded-full bg-gradient-to-r from-i17-cosmic to-i17-sky transition-all"
                        style={{ width: `${(doneCount / steps.length) * 100}%` }}
                    />
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                {steps.map((s) => (
                    <Step key={s.title} {...s} />
                ))}
            </GlassCard>

            <GlassCard className="p-5 text-sm text-muted-foreground">
                This checklist reflects live status from <span className="mono text-foreground">/api/v1/integrations</span>,{" "}
                <span className="mono text-foreground">/api/v1/packetwolf/status</span> and{" "}
                <span className="mono text-foreground">/api/v1/auth/providers</span> — it updates as you configure things in Settings.
            </GlassCard>
        </div>
    );
}
