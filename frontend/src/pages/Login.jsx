import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Lock, ShieldCheck, User, ArrowRight, ChevronLeft, CheckCircle, KeyRound } from "lucide-react";
import { useAuth } from "@/lib/auth";
import ZyvorMark from "@/components/ZyvorMark";
import { SSO, MFA } from "@/constants/testIds";
import { toast } from "sonner";
import { Trans, useTranslation } from "react-i18next";
import LanguageSelect from "@/components/LanguageSelect";
import {
    PremiumLoginShell,
    LoginField,
    LoginSubmit,
    LoginRemember,
    LoginError,
} from "@/components/PremiumLoginShell";

const SAVED_LOGIN_KEY = "axiom-saved-login";

const PILLS = [
    { id: "workloads", tone: "sky" },
    { id: "storage", tone: "violet" },
    { id: "networking", tone: "amber" },
    { id: "vms", tone: "emerald" },
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
    const { t } = useTranslation();
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
        toast.success(t("auth:login.welcomeBack", { name: username }), { description: t("auth:login.sessionEstablished") });
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
            setError(t("auth:login.badCredentials"));
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
            setError(useRecovery ? t("auth:login.badRecoveryCode") : t("auth:login.badCode"));
        } else {
            // The password was refused this time (or the account is locked out).
            setCode("");
            setPassword("");
            setStep("password");
            setError(t("auth:login.badCredentials"));
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
        document.title = t("auth:login.documentTitle", { host: loginDest.host || t("auth:login.documentTitleHostFallback") });
        return () => {
            document.title = "Axiom";
        };
    }, [loginDest.host, t]);

    // A single hero CTA, not h2kvm's redundant primary+secondary pair: both
    // buttons there just scroll to the same sign-in card. Text deliberately
    // avoids "Continue" and "Sign in" — the identity form's submit button
    // and the password form's "Sign in to Axiom" submit button already own
    // those names, and Axiom's own e2e suite locates them by exact/regex
    // role name, which breaks if this scroll-only button also matches.
    const heroCta = (
        <button type="button" className="login-cta-primary" onClick={scrollToSignIn}>
            {t("auth:login.getStarted")}
        </button>
    );

    const panelSubtitle =
        step === "mfa" ? (
            t("auth:login.twoFactor")
        ) : step === "password" ? (
            <Trans i18nKey="auth:login.enterPasswordFor" values={{ name: username.trim() }} components={{ host: <span className="login-apple-host" /> }} />
        ) : (
            t("auth:login.signIn")
        );

    const panelHint =
        step === "identity" && labHint ? (
            <>
                {t("auth:login.labMode")}{" "}
                <button type="button" onClick={fillDemo} className="login-hint-action">
                    {t("auth:login.fill", { hint: labHint })}
                </button>
            </>
        ) : null;

    return (
        <PremiumLoginShell
            data-testid={SSO.page}
            logo={<ZyvorMark size={40} idPrefix="axiom-login" className="axiom-login-mark" />}
            productName="Axiom"
            productWordmark="Axiom"
            heroTitle={t("auth:login.heroTitle")}
            heroSubheadline={
                loginDest.host
                    ? t("auth:login.heroSubheadlineHost", { host: loginDest.host })
                    : t("auth:login.heroSubheadline")
            }
            pills={PILLS.map((p) => ({ ...p, label: t(`auth:login.pills.${p.id}`) }))}
            heroCta={heroCta}
            chapterNote={t("auth:login.chapterNote")}
            panelSubtitle={panelSubtitle}
            panelHint={panelHint}
            footer={
                <div className="axiom-login-footer">
                    <p>{t("auth:login.footer")}</p>
                    <LanguageSelect className="mx-auto mt-2 w-fit" />
                </div>
            }
            showSignInChapter
        >
            {step === "mfa" ? (
                <form key="mfa" onSubmit={handleMfaSignIn} autoComplete="off" aria-label={t("auth:login.twoFactorCode")} className="login-apple-step text-left">
                    <button type="button" onClick={handleBack} className="login-apple-identity" aria-label={t("auth:login.changeAccount")}>
                        <ChevronLeft aria-hidden="true" className="h-4 w-4 shrink-0" />
                        <span className="truncate">{username.trim()}</span>
                    </button>

                    <p className="mb-3 text-sm text-muted-foreground">
                        {useRecovery
                            ? t("auth:login.recoveryPrompt")
                            : t("auth:login.codePrompt")}
                    </p>

                    {error ? <LoginError message={error} /> : null}

                    <div className="login-apple-fields">
                        <LoginField label={useRecovery ? t("auth:login.recoveryCode") : t("auth:login.authCode")} id="mfa-code">
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
                                <span>{t("auth:login.verifying")}</span>
                            </>
                        ) : (
                            <>
                                <span>{t("auth:login.verify")}</span>
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
                        {useRecovery ? t("auth:login.useApp") : t("auth:login.useRecovery")}
                    </button>
                </form>
            ) : step === "identity" ? (
                <form key="identity" onSubmit={handleContinue} autoComplete="on" aria-label={t("auth:login.account")} className="login-apple-step text-left">
                    <div className="login-apple-fields">
                        <LoginField label={t("auth:login.username")} id="username">
                            <User className="login-field-icon" />
                            <input
                                id="username"
                                name="username"
                                type="text"
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                                className="login-input"
                                placeholder={t("auth:login.username")}
                                autoComplete="username"
                                autoFocus
                                required
                            />
                        </LoginField>
                    </div>

                    <LoginSubmit disabled={!username.trim()}>
                        <span>{t("auth:login.continue")}</span>
                        <ArrowRight className="h-4 w-4" />
                    </LoginSubmit>
                </form>
            ) : (
                <form key="password" onSubmit={handleLocalSignIn} autoComplete="on" aria-label={t("auth:login.password")} className="login-apple-step text-left">
                    <button type="button" onClick={handleBack} className="login-apple-identity" aria-label={t("auth:login.changeAccount")}>
                        <ChevronLeft aria-hidden="true" className="h-4 w-4 shrink-0" />
                        <span className="truncate">{username.trim()}</span>
                    </button>
                    <input type="text" name="username" value={username} autoComplete="username" readOnly hidden />

                    {error ? <LoginError message={error} /> : null}

                    <div className="login-apple-fields">
                        <LoginField label={t("auth:login.password")} id="password">
                            <Lock className="login-field-icon" />
                            <input
                                id="password"
                                name="password"
                                type="password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                className="login-input"
                                placeholder={t("auth:login.password")}
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
                        label={t("auth:login.rememberMe")}
                    />

                    <LoginSubmit loading={pending} disabled={pending || !password}>
                        {pending ? (
                            <>
                                <Loader2 className="h-4 w-4 animate-spin" />
                                <span>{t("auth:login.signingIn")}</span>
                            </>
                        ) : (
                            <>
                                <span>{t("auth:login.submit")}</span>
                                <ArrowRight className="h-4 w-4" />
                            </>
                        )}
                    </LoginSubmit>

                    {oidcLoginUrl ? (
                        <a href={oidcLoginUrl} className="login-sso-link">
                            {t("auth:login.sso")}
                        </a>
                    ) : null}
                    {samlLoginUrl ? (
                        <a href={samlLoginUrl} className="login-sso-link">
                            <ShieldCheck className="h-3.5 w-3.5 mr-1.5" />
                            {t("auth:login.saml")}
                        </a>
                    ) : null}

                    <div className="flex items-center justify-center gap-1.5 mt-4 text-xs text-muted-foreground">
                        <CheckCircle className="h-3.5 w-3.5" />
                        <span>{t("auth:login.sessionNote")}</span>
                    </div>
                </form>
            )}
        </PremiumLoginShell>
    );
}
