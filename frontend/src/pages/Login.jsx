import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Lock, ShieldCheck, User, ArrowRight, ChevronLeft, CheckCircle, KeyRound } from "lucide-react";
import { useAuth } from "@/lib/auth";
import ZyvorMark from "@/components/ZyvorMark";
import { SSO, MFA } from "@/constants/testIds";
import { toast } from "sonner";
import {
    PremiumLoginShell,
    LoginField,
    LoginSubmit,
    LoginRemember,
    LoginError,
} from "@/components/PremiumLoginShell";

const SAVED_LOGIN_KEY = "axiom-saved-login";

const PILLS = [
    { label: "Workloads", tone: "sky" },
    { label: "Storage", tone: "violet" },
    { label: "Networking", tone: "amber" },
    { label: "Virtual Machines", tone: "emerald" },
];

function readSavedUsername() {
    try {
        const raw = localStorage.getItem(SAVED_LOGIN_KEY);
        if (!raw) return "";
        return JSON.parse(raw)?.username || "";
    } catch {
        return "";
    }
}

export default function Login() {
    const { signInLocal, providers, oidcLoginUrl, samlLoginUrl, labHint, defaultUsername } = useAuth();
    const navigate = useNavigate();
    const saved = useMemo(readSavedUsername, []);
    const [username, setUsername] = useState(saved || defaultUsername || "");
    const [password, setPassword] = useState("");
    const [pending, setPending] = useState(false);
    const [error, setError] = useState("");
    const [rememberMe, setRememberMe] = useState(!!saved);
    // Apple-style two-step sign-in: identity first, password second — the
    // actual signInLocal() call still sends both together on step 2 submit,
    // this only splits how they're collected.
    const [step, setStep] = useState(saved ? "password" : "identity");
    // Second step for accounts with two-factor sign-in: a 6-digit code from an
    // authenticator app, or one of the one-time recovery codes.
    const [code, setCode] = useState("");
    const [useRecovery, setUseRecovery] = useState(false);

    const clusterCtx = providers?.cluster || null;

    useEffect(() => {
        if (!saved && defaultUsername) setUsername((u) => u || defaultUsername);
    }, [saved, defaultUsername]);

    const scrollToSignIn = () => {
        document.getElementById("login-sign-in")?.scrollIntoView({ behavior: "smooth", block: "start" });
    };

    const handleContinue = (e) => {
        e.preventDefault();
        if (!username.trim()) return;
        setError("");
        setStep("password");
    };

    const handleBack = () => {
        setStep("identity");
        setPassword("");
        setCode("");
        setUseRecovery(false);
        setError("");
    };

    const finishSignIn = () => {
        if (rememberMe) {
            localStorage.setItem(SAVED_LOGIN_KEY, JSON.stringify({ username: username.trim() }));
        } else {
            localStorage.removeItem(SAVED_LOGIN_KEY);
        }
        toast.success(`Welcome back, ${username}`, { description: "Session established." });
        navigate("/overview", { replace: true });
    };

    const handleLocalSignIn = async (e) => {
        e.preventDefault();
        if (pending || !username.trim() || !password) return;
        setPending(true);
        setError("");
        const result = await signInLocal(username.trim(), password);
        setPending(false);
        if (result === "ok") {
            finishSignIn();
        } else if (result === "mfa") {
            setCode("");
            setUseRecovery(false);
            setStep("mfa");
        } else {
            setPassword("");
            setError("Check your username and password.");
        }
    };

    const handleMfaSignIn = async (e) => {
        e.preventDefault();
        if (pending || !code.trim()) return;
        setPending(true);
        setError("");
        const result = await signInLocal(username.trim(), password, code.trim());
        setPending(false);
        if (result === "ok") {
            finishSignIn();
        } else if (result === "mfa-denied") {
            setCode("");
            setError(useRecovery ? "That recovery code didn't work. Each one can only be used once." : "That code didn't work. Check the time on your device and try the next code.");
        } else {
            // The password was refused this time (or the account is locked out).
            setCode("");
            setPassword("");
            setStep("password");
            setError("Check your username and password.");
        }
    };

    const fillDemo = () => {
        setUsername("demo");
        setPassword("demo");
        setStep("password");
    };

    const loginDest = useMemo(() => {
        if (typeof window === "undefined") return { host: "", origin: "" };
        const { hostname, origin } = window.location;
        return { host: hostname || "localhost", origin: origin || "" };
    }, []);

    useEffect(() => {
        document.title = `Sign in · Axiom · ${loginDest.host || "cluster"}`;
        return () => {
            document.title = "Axiom";
        };
    }, [loginDest.host]);

    // A single hero CTA, not h2kvm's redundant primary+secondary pair: both
    // buttons there just scroll to the same sign-in card. Text deliberately
    // avoids "Continue" and "Sign in" — the identity form's submit button
    // and the password form's "Sign in to Axiom" submit button already own
    // those names, and Axiom's own e2e suite locates them by exact/regex
    // role name, which breaks if this scroll-only button also matches.
    const heroCta = (
        <button type="button" className="login-cta-primary" onClick={scrollToSignIn}>
            Get started
        </button>
    );

    const panelSubtitle =
        step === "mfa" ? (
            "Two-factor authentication"
        ) : step === "password" ? (
            <>
                Enter the password for <span className="login-apple-host">{username.trim()}</span>
            </>
        ) : (
            "Sign in"
        );

    const panelHint =
        step === "identity" && labHint ? (
            <>
                Lab mode — no SSO configured.{" "}
                <button type="button" onClick={fillDemo} className="login-hint-action">
                    Fill {labHint}
                </button>
            </>
        ) : null;

    return (
        <PremiumLoginShell
            data-testid={SSO.page}
            logo={<ZyvorMark size={40} idPrefix="axiom-login" className="axiom-login-mark" />}
            productName="Axiom"
            productWordmark="Axiom"
            heroTitle="Sign in to Axiom."
            heroSubheadline={
                loginDest.host
                    ? `Kubernetes-native private cloud on ${loginDest.host}.`
                    : "Your Kubernetes-native private cloud console."
            }
            pills={PILLS}
            heroCta={heroCta}
            chapterNote="Local account or SSO · sign in to continue"
            panelSubtitle={panelSubtitle}
            panelHint={panelHint}
            footer={<p className="axiom-login-footer">Axiom by Zyvor · zyvor.dev</p>}
            showSignInChapter
        >
            {step === "mfa" ? (
                <form key="mfa" onSubmit={handleMfaSignIn} autoComplete="off" aria-label="Two-factor code" className="login-apple-step text-left">
                    <button type="button" onClick={handleBack} className="login-apple-identity" aria-label="Change account">
                        <ChevronLeft aria-hidden="true" className="h-4 w-4 shrink-0" />
                        <span className="truncate">{username.trim()}</span>
                    </button>

                    <p className="mb-3 text-sm text-muted-foreground">
                        {useRecovery
                            ? "Enter one of your recovery codes. Each code works once."
                            : "Enter the 6-digit code from your authenticator app."}
                    </p>

                    {error ? <LoginError message={error} /> : null}

                    <div className="login-apple-fields">
                        <LoginField label={useRecovery ? "Recovery code" : "Authentication code"} id="mfa-code">
                            <KeyRound className="login-field-icon" />
                            <input
                                id="mfa-code"
                                name="mfa-code"
                                data-testid={MFA.loginCode}
                                type="text"
                                inputMode={useRecovery ? "text" : "numeric"}
                                value={code}
                                onChange={(e) => setCode(e.target.value)}
                                className="login-input"
                                placeholder={useRecovery ? "xxxxx-xxxxx" : "123456"}
                                autoComplete="one-time-code"
                                autoFocus
                                required
                                disabled={pending}
                            />
                        </LoginField>
                    </div>

                    <LoginSubmit loading={pending} disabled={pending || !code.trim()} data-testid={MFA.loginSubmit}>
                        {pending ? (
                            <>
                                <Loader2 className="h-4 w-4 animate-spin" />
                                <span>Verifying…</span>
                            </>
                        ) : (
                            <>
                                <span>Verify</span>
                                <ArrowRight className="h-4 w-4" />
                            </>
                        )}
                    </LoginSubmit>

                    <button
                        type="button"
                        className="login-sso-link"
                        data-testid={useRecovery ? MFA.loginUseApp : MFA.loginUseRecovery}
                        onClick={() => {
                            setUseRecovery((v) => !v);
                            setCode("");
                            setError("");
                        }}
                    >
                        {useRecovery ? "Use my authenticator app instead" : "Use a recovery code instead"}
                    </button>
                </form>
            ) : step === "identity" ? (
                <form key="identity" onSubmit={handleContinue} autoComplete="on" aria-label="Account" className="login-apple-step text-left">
                    <div className="login-apple-fields">
                        <LoginField label="Username" id="username">
                            <User className="login-field-icon" />
                            <input
                                id="username"
                                name="username"
                                type="text"
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                                className="login-input"
                                placeholder="Username"
                                autoComplete="username"
                                autoFocus
                                required
                            />
                        </LoginField>
                    </div>

                    <LoginSubmit disabled={!username.trim()}>
                        <span>Continue</span>
                        <ArrowRight className="h-4 w-4" />
                    </LoginSubmit>
                </form>
            ) : (
                <form key="password" onSubmit={handleLocalSignIn} autoComplete="on" aria-label="Password" className="login-apple-step text-left">
                    <button type="button" onClick={handleBack} className="login-apple-identity" aria-label="Change account">
                        <ChevronLeft aria-hidden="true" className="h-4 w-4 shrink-0" />
                        <span className="truncate">{username.trim()}</span>
                    </button>
                    <input type="text" name="username" value={username} autoComplete="username" readOnly hidden />

                    {error ? <LoginError message={error} /> : null}

                    <div className="login-apple-fields">
                        <LoginField label="Password" id="password">
                            <Lock className="login-field-icon" />
                            <input
                                id="password"
                                name="password"
                                type="password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                className="login-input"
                                placeholder="Password"
                                autoComplete="current-password"
                                autoFocus
                                required
                                disabled={pending}
                            />
                        </LoginField>
                    </div>

                    <LoginRemember
                        checked={rememberMe}
                        onChange={(checked) => {
                            setRememberMe(checked);
                            if (!checked) localStorage.removeItem(SAVED_LOGIN_KEY);
                        }}
                        label="Remember me on this device"
                    />

                    <LoginSubmit loading={pending} disabled={pending || !password}>
                        {pending ? (
                            <>
                                <Loader2 className="h-4 w-4 animate-spin" />
                                <span>Signing in…</span>
                            </>
                        ) : (
                            <>
                                <span>Sign in to Axiom</span>
                                <ArrowRight className="h-4 w-4" />
                            </>
                        )}
                    </LoginSubmit>

                    {oidcLoginUrl ? (
                        <a href={oidcLoginUrl} className="login-sso-link">
                            Sign in with SSO
                        </a>
                    ) : null}
                    {samlLoginUrl ? (
                        <a href={samlLoginUrl} className="login-sso-link">
                            <ShieldCheck className="h-3.5 w-3.5 mr-1.5" />
                            Sign in with SAML
                        </a>
                    ) : null}

                    <div className="flex items-center justify-center gap-1.5 mt-4 text-xs text-muted-foreground">
                        <CheckCircle className="h-3.5 w-3.5" />
                        <span>Axiom sessions are scoped per device and expire automatically.</span>
                    </div>
                </form>
            )}
        </PremiumLoginShell>
    );
}
