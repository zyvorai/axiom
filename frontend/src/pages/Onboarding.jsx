import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import PageHeader from "@/components/PageHeader";
import { usePageTitle } from "@/lib/pageTitle";
import { Check, ShieldCheck, Cloudy, Radar, KeyRound, Database, Activity, ArrowRight } from "lucide-react";

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

export default function Onboarding() {
    usePageTitle("Getting started");
    const status = useSetupStatus();
    const { user } = useAuth();

    if (!status) {
        return (
            <div className="page">
                <PageHeader title="Getting started" />
                <div className="card empty">Loading…</div>
            </div>
        );
    }

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
                ? "OIDC or SAML is configured — the SSO links on the login screen are live."
                : "No OIDC/SAML provider configured yet. Local username and password sign-in works, but SSO is off.",
            to: "/settings",
            cta: "Configure",
        },
        {
            done: !usingDemoDefaults,
            required: true,
            icon: KeyRound,
            title: "Move off the demo credential",
            description: usingDemoDefaults
                ? "This instance runs with lab defaults. Set AXIOM_USERNAME and AXIOM_PASSWORD, or connect SSO, before it leaves the lab."
                : "Not running with lab default credentials.",
            to: usingDemoDefaults ? "/settings" : null,
            cta: "Fix now",
        },
        {
            done: !!integrations?.atlasConfigured,
            required: false,
            icon: Cloudy,
            title: "Connect Atlas storage",
            description: integrations?.atlasConfigured
                ? "Atlas is wired — the Atlas page shows live backends, pools and volumes."
                : "No storage control plane connected. Atlas shows volumes and pools and lets you provision storage once configured.",
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
                    ? "PacketWolf is connected. Live eBPF flow data shows on the Networking page."
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
            description: "Only needed for Postgres-backed features. Configure host and credentials in Settings.",
            to: "/settings",
            cta: "Configure",
        },
        {
            done: !!integrations?.probeConfigured,
            required: false,
            icon: Activity,
            title: "Connect an external probe",
            description: "A synthetic-check target for platform health. Configure in Settings.",
            to: "/settings",
            cta: "Configure",
        },
    ];

    const doneCount = steps.filter((s) => s.done).length;

    return (
        <div className="page rhythm">
            <section className="hero hero-media">
                <h1>Getting started</h1>
                <p>{`${user ? `Welcome, ${user.name}. ` : ""}Six steps to take this cluster from lab defaults to something you'd trust in production. Two are required.`}</p>
                <div className="progress-row mx-auto mt-8 max-w-[420px]" role="group" aria-label="Setup progress">
                    <span>
                        {doneCount} of {steps.length} complete
                    </span>
                    <div className="meter">
                        <i style={{ width: `${(doneCount / steps.length) * 100}%` }} />
                    </div>
                </div>
            </section>
            <div className="card media">
                <div>
                    {steps.map((s, i) => (
                        <div key={s.title} className={`check ${s.done ? "done" : ""}`}>
                            <div className="n">{s.done ? <Check size={15} /> : i + 1}</div>
                            <div className="body">
                                <h3>
                                    <s.icon size={17} />
                                    {s.title}
                                    {!s.required && <span className="pill soft">Optional</span>}
                                    {s.done && (
                                        <span className="pill" style={{ "--c": "var(--green)" }}>
                                            <i aria-hidden="true" />
                                            Done
                                        </span>
                                    )}
                                </h3>
                                <p>{s.description}</p>
                            </div>
                            {!s.done && s.to && (
                                <div className="act">
                                    <Link to={s.to} className={`btn ${s.required ? "btn-fill" : "btn-line"}`}>
                                        {s.cta}
                                        <ArrowRight size={14} />
                                    </Link>
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            </div>
            <p className="text-[13px] text-ink-3 mt-[18px]">
                Status is live from <span className="mono">/api/v1/integrations</span>, <span className="mono">/api/v1/packetwolf/status</span>{" "}
                and <span className="mono">/api/v1/auth/providers</span>.
            </p>
        </div>
    );
}
