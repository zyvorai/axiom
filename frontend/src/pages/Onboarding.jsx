import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
    ShieldCheck,
    Cloudy,
    Radar,
    KeyRound,
    Database,
    Activity,
    CheckCircle2,
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

function Step({ index, done, required, icon: Icon, title, description, to, cta }) {
    return (
        <div className="flex items-start gap-5 border-b px-6 py-6 last:border-none">
            <div
                className={`mono grid h-10 w-10 shrink-0 place-items-center rounded-full text-sm font-semibold ${
                    done ? "bg-emerald-500/15 text-emerald-300" : "bg-foreground/[0.06] text-muted-foreground"
                }`}
            >
                {done ? <CheckCircle2 className="h-5 w-5" /> : String(index + 1).padStart(2, "0")}
            </div>
            <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                    <Icon className="h-4 w-4 shrink-0 text-i17-cosmic" />
                    <div className="text-base font-semibold tracking-tight">{title}</div>
                    {!required && (
                        <Badge variant="outline" className="bg-muted/30 text-[10px] text-muted-foreground">
                            optional
                        </Badge>
                    )}
                    {done && (
                        <Badge className="border-emerald-500/40 bg-emerald-500/10 text-[10px] text-emerald-300 hover:bg-emerald-500/10">
                            done
                        </Badge>
                    )}
                </div>
                <p className="mt-1 max-w-xl text-sm text-muted-foreground">{description}</p>
            </div>
            {!done && to && (
                <Button asChild variant="outline" size="sm" className="mt-0.5 shrink-0 rounded-full bg-muted/30">
                    <Link to={to}>
                        {cta || "Configure"} <ArrowRight className="ml-1 h-3 w-3" />
                    </Link>
                </Button>
            )}
        </div>
    );
}

export default function Onboarding() {
    const status = useSetupStatus();
    const { user } = useAuth();

    if (!status) return <Skeleton className="h-96 rounded-2xl bg-muted" />;

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
        <div className="space-y-6">
            <PageHeader
                eyebrow="GETTING STARTED"
                title="Getting Started"
                description={`${user ? `Welcome, ${user.name}. ` : ""}What's configured on this Axiom instance, and what's left to wire up.`}
            />

            <GlassCard className="p-6">
                <div className="flex items-center justify-between">
                    <div>
                        <div className="text-sm font-medium">Setup progress</div>
                        <div className="mt-0.5 text-xs text-muted-foreground">
                            {requiredDone}/{requiredSteps.length} required · {doneCount}/{steps.length} total
                        </div>
                    </div>
                    <div className="text-3xl font-bold tabular-nums">{Math.round((doneCount / steps.length) * 100)}%</div>
                </div>
                <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-muted">
                    <div
                        className="h-full rounded-full bg-gradient-to-r from-i17-cosmic to-i17-sky transition-all"
                        style={{ width: `${(doneCount / steps.length) * 100}%` }}
                    />
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                {steps.map((s, i) => (
                    <Step key={s.title} index={i} {...s} />
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
