import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import PageHeader from "@/components/PageHeader";
import { usePageTitle } from "@/lib/pageTitle";
import { useTranslation, Trans } from "react-i18next";
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
    const { t } = useTranslation();
    usePageTitle(t("pages:onboarding.title"));
    const status = useSetupStatus();
    const { user } = useAuth();

    if (!status) {
        return (
            <div className="page">
                <PageHeader title={t("pages:onboarding.title")} />
                <div className="card empty">{t("common:states.loading")}</div>
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
            title: t("pages:onboarding.sso.title"),
            description: ssoConfigured ? t("pages:onboarding.sso.done") : t("pages:onboarding.sso.todo"),
            to: "/settings",
            cta: t("pages:onboarding.configure"),
        },
        {
            done: !usingDemoDefaults,
            required: true,
            icon: KeyRound,
            title: t("pages:onboarding.demo.title"),
            description: usingDemoDefaults ? t("pages:onboarding.demo.todo") : t("pages:onboarding.demo.done"),
            to: usingDemoDefaults ? "/settings" : null,
            cta: t("pages:onboarding.fixNow"),
        },
        {
            done: !!integrations?.atlasConfigured,
            required: false,
            icon: Cloudy,
            title: t("pages:onboarding.atlas.title"),
            description: integrations?.atlasConfigured ? t("pages:onboarding.atlas.done") : t("pages:onboarding.atlas.todo"),
            to: "/atlas",
            cta: t("pages:onboarding.atlas.cta"),
        },
        {
            done: !!packetwolf?.configured,
            required: false,
            icon: Radar,
            title: t("pages:onboarding.packetwolf.title"),
            description: packetwolf?.configured
                ? packetwolf.reachable
                    ? t("pages:onboarding.packetwolf.connected")
                    : t("pages:onboarding.packetwolf.unreachable")
                : t("pages:onboarding.packetwolf.todo"),
            to: "/networking",
            cta: t("pages:onboarding.packetwolf.cta"),
        },
        {
            done: !!integrations?.postgresConfigured,
            required: false,
            icon: Database,
            title: t("pages:onboarding.postgres.title"),
            description: t("pages:onboarding.postgres.description"),
            to: "/settings",
            cta: t("pages:onboarding.configure"),
        },
        {
            done: !!integrations?.probeConfigured,
            required: false,
            icon: Activity,
            title: t("pages:onboarding.probe.title"),
            description: t("pages:onboarding.probe.description"),
            to: "/settings",
            cta: t("pages:onboarding.configure"),
        },
    ];

    const doneCount = steps.filter((s) => s.done).length;

    return (
        <div className="page rhythm">
            <section className="hero hero-media">
                <h1>{t("pages:onboarding.title")}</h1>
                <p>{user ? t("pages:onboarding.introWelcome", { name: user.name }) : t("pages:onboarding.intro")}</p>
                <div className="progress-row mx-auto mt-8 max-w-[420px]" role="group" aria-label={t("pages:onboarding.progressLabel")}>
                    <span>
                        {t("pages:onboarding.progress", { done: doneCount, total: steps.length })}
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
                                <h2>
                                    <s.icon size={17} />
                                    {s.title}
                                    {!s.required && <span className="pill soft">{t("pages:onboarding.optional")}</span>}
                                    {s.done && (
                                        <span className="pill" style={{ "--c": "var(--green)" }}>
                                            <i aria-hidden="true" />
                                            {t("common:actions.done")}
                                        </span>
                                    )}
                                </h2>
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
                <Trans i18nKey="pages:onboarding.statusSource" components={{ code: <span className="mono" /> }} />
            </p>
        </div>
    );
}
