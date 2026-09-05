import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Lock, ShieldCheck, KeyRound, ArrowRight, ChevronLeft } from "lucide-react";
import { useAuth } from "@/lib/auth";
import ZyvorMark from "@/components/ZyvorMark";
import { SSO } from "@/constants/testIds";
import { toast } from "sonner";
import "../axiom-login.css";

const GoogleIcon = () => (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <path fill="#4285F4" d="M23.5 12.27c0-.85-.08-1.66-.22-2.45H12v4.64h6.45a5.52 5.52 0 0 1-2.39 3.62v3h3.87c2.26-2.09 3.57-5.16 3.57-8.81Z" />
        <path fill="#34A853" d="M12 24c3.24 0 5.96-1.07 7.93-2.91l-3.87-3c-1.07.72-2.44 1.14-4.06 1.14-3.12 0-5.77-2.11-6.72-4.95H1.3v3.1A12 12 0 0 0 12 24Z" />
        <path fill="#FBBC05" d="M5.28 14.28a7.2 7.2 0 0 1 0-4.56v-3.1H1.3a12 12 0 0 0 0 10.76l3.98-3.1Z" />
        <path fill="#EA4335" d="M12 4.77c1.76 0 3.34.6 4.58 1.79l3.44-3.44A11.98 11.98 0 0 0 1.3 6.62l3.98 3.1C6.23 6.88 8.88 4.77 12 4.77Z" />
    </svg>
);

const GitHubIcon = () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        <path d="M12 .5A11.5 11.5 0 0 0 .5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-2.17c-3.2.7-3.87-1.37-3.87-1.37-.52-1.33-1.28-1.68-1.28-1.68-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.19 1.76 1.19 1.03 1.75 2.69 1.25 3.35.95.1-.74.4-1.25.72-1.54-2.55-.29-5.24-1.28-5.24-5.68 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11.05 11.05 0 0 1 5.8 0C17.85 4.9 18.82 5.2 18.82 5.2c.63 1.6.23 2.77.11 3.06.74.8 1.18 1.83 1.18 3.09 0 4.41-2.69 5.38-5.26 5.66.41.36.78 1.05.78 2.13v3.16c0 .31.2.68.8.56A11.5 11.5 0 0 0 23.5 12 11.5 11.5 0 0 0 12 .5Z" />
    </svg>
);

const OktaIcon = () => (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="10" fill="none" stroke="#007DC1" strokeWidth="4.5" />
    </svg>
);

const SSO_BUTTONS = [
    { id: "google", label: "Continue with Google", Icon: GoogleIcon, testId: SSO.google },
    { id: "github", label: "Continue with GitHub", Icon: GitHubIcon, testId: SSO.github },
    { id: "okta", label: "Continue with Okta", Icon: OktaIcon, testId: SSO.okta },
];

export default function Login() {
    const { signInLocal, providers, oidcLoginUrl, samlLoginUrl, labHint, defaultUsername } = useAuth();
    const navigate = useNavigate();
    const [username, setUsername] = useState(defaultUsername || "");
    const [password, setPassword] = useState("");
    const [pending, setPending] = useState(false);
    const [shake, setShake] = useState(false);
    // Apple-style two-step sign-in: identity first, password second — the
    // actual signInLocal() call still sends both together on step 2 submit,
    // this only splits how they're collected.
    const [step, setStep] = useState("identity");

    const clusterCtx = providers?.cluster || null;

    useEffect(() => {
        if (defaultUsername) setUsername((u) => u || defaultUsername);
    }, [defaultUsername]);

    const handleContinue = (e) => {
        e.preventDefault();
        if (!username) return;
        setStep("password");
    };

    const handleBack = () => {
        setStep("identity");
        setPassword("");
    };

    const handleLocalSignIn = async (e) => {
        e.preventDefault();
        if (pending || !username || !password) return;
        setPending(true);
        const ok = await signInLocal(username, password);
        setPending(false);
        if (ok) {
            toast.success(`Welcome back, ${username}`, { description: "Session established." });
            navigate("/overview", { replace: true });
        } else {
            setPassword("");
            setShake(true);
            setTimeout(() => setShake(false), 450);
            toast.error("Sign in failed", { description: "Check your username and password." });
        }
    };

    const fillDemo = () => {
        setUsername("demo");
        setPassword("demo");
        setStep("password");
    };

    const handleSSO = () => {
        if (oidcLoginUrl) {
            window.location.href = oidcLoginUrl;
            return;
        }
        toast.info("SSO not configured", {
            description: labHint
                ? `No OIDC provider is set up yet — sign in below with ${labHint}.`
                : "Configure OIDC in Settings → Integrations, or sign in with a local account below.",
        });
    };

    const loginDest = useMemo(() => {
        if (typeof window === "undefined") return { host: "", origin: "", port: "", protocol: "" };
        const { hostname, origin, port, protocol } = window.location;
        return {
            host: hostname || "localhost",
            origin: origin || "",
            port: port || (protocol === "https:" ? "443" : protocol === "http:" ? "80" : ""),
            protocol: protocol.replace(":", "") || "https",
        };
    }, []);

    useEffect(() => {
        document.title = `Sign in · Axiom · ${loginDest.host || "cluster"}`;
        return () => {
            document.title = "Axiom";
        };
    }, [loginDest.host]);

    const scrollToChapter = (id) => {
        document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
    };

    return (
        <div data-testid={SSO.page} className="axiom-login-page login-page login-store-page">
            <nav className="login-localnav" aria-label="Login chapters">
                <a href="#login-product" onClick={(e) => { e.preventDefault(); scrollToChapter("login-product"); }}>Product</a>
                <a href="#login-machine" onClick={(e) => { e.preventDefault(); scrollToChapter("login-machine"); }}>This machine</a>
                <a href="#login-sign-in" onClick={(e) => { e.preventDefault(); scrollToChapter("login-sign-in"); }}>Sign in</a>
                {loginDest.host ? (
                    <span className="login-localnav-host" title={loginDest.origin}>{loginDest.host}</span>
                ) : null}
            </nav>

            <main className="login-store-scroll" aria-label="Sign in">
            <section id="login-product" className="login-chapter login-chapter-hero axiom-login-brand">
                <ZyvorMark size={56} idPrefix="axiom-login" className="axiom-login-mark" />
                <p className="axiom-login-product">Axiom</p>
                <p className="axiom-login-product-by">by Zyvor</p>
                <h1 className="axiom-login-hero-title">Sign in to Axiom.</h1>
                <p className="axiom-login-tagline">
                    {loginDest.host
                        ? `Kubernetes-native private cloud on ${loginDest.host}. Confirm the machine below, then sign in.`
                        : "Your Kubernetes-native private cloud console."}
                </p>
                <div className="login-cta" style={{ marginTop: "1.5rem" }}>
                    <button type="button" className="login-cta-primary" onClick={() => scrollToChapter("login-machine")}>
                        This machine
                    </button>
                    <button type="button" className="login-cta-secondary" onClick={() => scrollToChapter("login-sign-in")}>
                        Sign in
                    </button>
                </div>
            </section>

            <section id="login-machine" className="login-chapter login-chapter-destination" aria-label="This machine">
                <div className="login-chapter-inner">
                    <p className="login-chapter-kicker">This machine</p>
                    <h2 className="login-dest-title">{loginDest.host || "localhost"}.</h2>
                    <p className="login-tagline">
                        You are signing in to <strong style={{ color: "#fff", fontWeight: 600 }}>Axiom</strong> on
                        this host — not a public cloud console.
                    </p>
                    <ul className="login-dest-facts">
                        <li className="login-dest-fact">
                            <span className="login-dest-fact-label">Product</span>
                            <span className="login-dest-fact-value is-display">Axiom</span>
                        </li>
                        <li className="login-dest-fact">
                            <span className="login-dest-fact-label">Host</span>
                            <span className="login-dest-fact-value">{loginDest.host || "—"}</span>
                        </li>
                        <li className="login-dest-fact">
                            <span className="login-dest-fact-label">Origin</span>
                            <span className="login-dest-fact-value">{loginDest.origin || "—"}</span>
                        </li>
                        <li className="login-dest-fact">
                            <span className="login-dest-fact-label">Protocol</span>
                            <span className="login-dest-fact-value">
                                {loginDest.protocol || "—"}
                                {loginDest.port ? ` · ${loginDest.port}` : ""}
                            </span>
                        </li>
                        <li className="login-dest-fact">
                            <span className="login-dest-fact-label">Mode</span>
                            <span className="login-dest-fact-value">{clusterCtx?.mode || "—"}</span>
                        </li>
                        <li className="login-dest-fact">
                            <span className="login-dest-fact-label">kubectl context</span>
                            <span className="login-dest-fact-value" title={clusterCtx?.label || undefined}>
                                {clusterCtx?.context || "—"}
                                {clusterCtx?.contexts > 1 ? ` · ${clusterCtx.contexts} contexts` : ""}
                            </span>
                        </li>
                        {clusterCtx?.server ? (
                            <li className="login-dest-fact">
                                <span className="login-dest-fact-label">API server</span>
                                <span className="login-dest-fact-value">{clusterCtx.server}</span>
                            </li>
                        ) : null}
                        {clusterCtx?.label && clusterCtx.label !== clusterCtx.context ? (
                            <li className="login-dest-fact">
                                <span className="login-dest-fact-label">Cluster label</span>
                                <span className="login-dest-fact-value is-display">{clusterCtx.label}</span>
                            </li>
                        ) : null}
                    </ul>
                    <div className="login-cta">
                        <button type="button" className="login-cta-primary" onClick={() => scrollToChapter("login-sign-in")}>
                            Continue to sign in
                        </button>
                    </div>
                </div>
            </section>

            <section id="login-sign-in" className="login-chapter login-chapter-sign-in">
            <div className={`axiom-login-card${shake ? " axiom-login-shake" : ""}`}>
                <p className="login-sign-in-context">
                    Signing in to <strong>Axiom</strong>
                    {loginDest.host ? (
                        <>
                            {" "}
                            on <span className="login-apple-host">{loginDest.host}</span>
                        </>
                    ) : null}
                    {clusterCtx?.context ? (
                        <>
                            {" "}
                            · context <span className="login-apple-host">{clusterCtx.context}</span>
                            {clusterCtx.mode ? (
                                <span className="login-sign-in-mode"> ({clusterCtx.mode})</span>
                            ) : null}
                        </>
                    ) : null}
                </p>
                {step === "identity" ? (
                    <div key="identity">
                        <div className="axiom-login-eyebrow">Axiom account</div>
                        <h2 className="axiom-login-form-heading">Choose how to sign in</h2>
                        <p className="axiom-login-form-hint">Use SSO from your organization, or a local Axiom account.</p>

                        <div className="mt-6 space-y-2.5">
                            {SSO_BUTTONS.map(({ id, label, Icon, testId }) => (
                                <button
                                    key={id}
                                    data-testid={testId}
                                    onClick={handleSSO}
                                    className="axiom-login-btn-secondary"
                                >
                                    <span className="axiom-login-icon-chip">
                                        <Icon />
                                    </span>
                                    <span>{label}</span>
                                </button>
                            ))}
                            {samlLoginUrl && (
                                <a href={samlLoginUrl} className="axiom-login-btn-secondary">
                                    <span className="axiom-login-icon-chip">
                                        <ShieldCheck className="h-4 w-4" style={{ color: "hsl(211 100% 45%)" }} />
                                    </span>
                                    <span>Continue with SAML</span>
                                </a>
                            )}
                        </div>

                        <div className="axiom-login-divider">
                            <span>or use an Axiom account</span>
                        </div>

                        <form onSubmit={handleContinue}>
                            <input
                                autoFocus
                                placeholder="Username"
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                                className="axiom-login-input"
                            />
                            <button type="submit" disabled={!username} className="axiom-login-btn-primary">
                                Continue
                                <ArrowRight className="h-4 w-4" />
                            </button>

                            {labHint && (
                                <button
                                    type="button"
                                    onClick={fillDemo}
                                    className="axiom-login-hint mt-3 w-full underline-offset-2 hover:underline"
                                >
                                    Lab mode — fill {labHint}
                                </button>
                            )}
                        </form>
                    </div>
                ) : (
                    <div key="password">
                        <div className="axiom-login-eyebrow">Axiom account</div>
                        <h2 className="axiom-login-form-heading">Enter your password</h2>
                        <p className="axiom-login-form-hint">Signing in to Axiom as {username}.</p>

                        <button type="button" onClick={handleBack} className="axiom-login-btn-secondary mt-4">
                            <span
                                className="axiom-login-icon-chip"
                                style={{ background: "hsl(211 100% 45%)", border: "none", color: "#fff", fontWeight: 600, fontSize: "0.75rem" }}
                            >
                                {username.slice(0, 1).toUpperCase()}
                            </span>
                            <span className="min-w-0 flex-1 truncate">{username}</span>
                            <span className="axiom-login-hint flex shrink-0 items-center gap-1">
                                <ChevronLeft className="h-3.5 w-3.5" /> Not you?
                            </span>
                        </button>

                        <form className="mt-4" onSubmit={handleLocalSignIn}>
                            <input
                                autoFocus
                                type="password"
                                placeholder="Password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                className="axiom-login-input"
                            />
                            <button
                                type="submit"
                                disabled={pending || !username || !password}
                                className="axiom-login-btn-primary"
                            >
                                {pending ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}
                                {pending ? "Signing in…" : "Sign in to Axiom"}
                            </button>
                        </form>
                    </div>
                )}

                <div className="axiom-login-security-note">
                    <Lock className="h-3.5 w-3.5" style={{ color: "hsl(211 100% 45%)" }} />
                    Axiom sessions are scoped per device and expire automatically.
                </div>
            </div>
            </section>
            </main>

            <p className="axiom-login-footer">Axiom by Zyvor · zyvor.dev</p>
        </div>
    );
}

