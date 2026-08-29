import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Lock, ShieldCheck, Globe2, Cpu, KeyRound } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { Input } from "@/components/ui/input";
import { SSO } from "@/constants/testIds";
import { toast } from "sonner";

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

const STATS = [
    { icon: Globe2, label: "Multi-region", sub: "single cluster" },
    { icon: Cpu, label: "Live", sub: "workload data" },
    { icon: ShieldCheck, label: "mTLS", sub: "everywhere" },
];

export default function Login() {
    const { signInLocal, providers, oidcLoginUrl, samlLoginUrl, labHint, defaultUsername } = useAuth();
    const navigate = useNavigate();
    const [username, setUsername] = useState(defaultUsername || "");
    const [password, setPassword] = useState("");
    const [pending, setPending] = useState(false);

    useEffect(() => {
        if (defaultUsername) setUsername((u) => u || defaultUsername);
    }, [defaultUsername]);

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
            toast.error("Sign in failed", { description: "Check your username and password." });
        }
    };

    const fillDemo = () => {
        setUsername("demo");
        setPassword("demo");
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

    return (
        <div data-testid={SSO.page} className="relative flex min-h-screen items-center overflow-hidden">
            <div className="pointer-events-none absolute inset-0 -z-10">
                <div className="absolute inset-0 grid-bg opacity-40" />
                <div className="aurora aurora-a" />
                <div className="aurora aurora-b" />
                <div className="aurora aurora-c" />
            </div>

            <div className="mx-auto grid w-full max-w-6xl items-center gap-12 px-6 py-16 lg:grid-cols-[1.15fr_1fr] lg:gap-20">
                {/* Brand panel */}
                <div className="hidden lg:block">
                    <div className="fade-up flex items-center gap-3" style={{ animationDelay: "0ms" }}>
                        <div className="z-mark grid h-12 w-12 place-items-center rounded-2xl bg-gradient-to-br from-i17-cosmic via-[#f26b3a] to-[#c8452a] shadow-xl shadow-[#f26b3a]/40 ring-1 ring-white/15">
                            <svg width="24" height="24" viewBox="0 0 18 18" aria-hidden="true">
                                <path d="M2 2h14L6.6 16H16" fill="none" stroke="white" strokeWidth="2.6" strokeLinejoin="round" strokeLinecap="round" />
                            </svg>
                        </div>
                        <span className="text-lg font-semibold tracking-tight lowercase">zyvor</span>
                    </div>

                    <h1 className="fade-up mt-10 text-5xl font-semibold leading-[1.05] tracking-tight lg:text-6xl" style={{ animationDelay: "90ms" }}>
                        Axiom.
                        <br />
                        <span className="bg-gradient-to-r from-i17-cosmic via-i17-gold to-i17-sky bg-clip-text text-transparent">
                            Every cluster, one truth.
                        </span>
                    </h1>
                    <p className="fade-up mt-6 max-w-md text-base text-muted-foreground" style={{ animationDelay: "180ms" }}>
                        The Zyvor control plane for Kubernetes, Cilium, KubeVirt and multi-tenant SaaS —
                        rendered in liquid glass.
                    </p>

                    <div className="fade-up mt-10 flex gap-3" style={{ animationDelay: "270ms" }}>
                        {STATS.map(({ icon: Icon, label, sub }) => (
                            <div key={label} className="glass specular flex items-center gap-2.5 rounded-2xl px-4 py-3">
                                <Icon className="h-4 w-4 text-i17-cosmic" />
                                <div className="leading-tight">
                                    <div className="text-sm font-medium">{label}</div>
                                    <div className="text-[11px] text-muted-foreground">{sub}</div>
                                </div>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Sign-in card */}
                <div className="fade-up glass specular noise relative rounded-3xl p-8 sm:p-10" style={{ animationDelay: "140ms" }}>
                    <div className="mb-8 flex items-center gap-3 lg:hidden">
                        <div className="grid h-10 w-10 place-items-center rounded-xl bg-gradient-to-br from-i17-cosmic to-[#c8452a] ring-1 ring-white/15">
                            <svg width="20" height="20" viewBox="0 0 18 18" aria-hidden="true">
                                <path d="M2 2h14L6.6 16H16" fill="none" stroke="white" strokeWidth="2.6" strokeLinejoin="round" strokeLinecap="round" />
                            </svg>
                        </div>
                        <span className="text-base font-semibold lowercase">zyvor · Axiom</span>
                    </div>

                    <div className="text-[11px] uppercase tracking-[0.24em] text-i17-cosmic/90">Axiom Console</div>
                    <h2 className="mt-2 text-2xl font-semibold tracking-tight">Sign in to continue</h2>
                    <p className="mt-1.5 text-sm text-muted-foreground">
                        Single sign-on enforced by your Zyvor organization.
                    </p>

                    <div className="mt-8 space-y-3">
                        {SSO_BUTTONS.map(({ id, label, Icon, testId }, i) => (
                            <button
                                key={id}
                                data-testid={testId}
                                onClick={handleSSO}
                                className="sso-btn group flex w-full items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3.5 text-sm font-medium backdrop-blur-md transition-colors hover:bg-white/[0.09]"
                                style={{ animationDelay: `${i * 60}ms` }}
                            >
                                <span className="grid h-8 w-8 place-items-center rounded-xl bg-white/[0.06] ring-1 ring-white/10">
                                    <Icon />
                                </span>
                                <span>{label}</span>
                                <span className="ml-auto text-[10px] uppercase tracking-widest text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100">
                                    SSO
                                </span>
                            </button>
                        ))}
                        {samlLoginUrl && (
                            <a
                                href={samlLoginUrl}
                                className="flex w-full items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3.5 text-sm font-medium backdrop-blur-md transition-colors hover:bg-white/[0.09]"
                            >
                                <span className="grid h-8 w-8 place-items-center rounded-xl bg-white/[0.06] ring-1 ring-white/10">
                                    <ShieldCheck className="h-4 w-4 text-i17-sage" />
                                </span>
                                <span>Continue with SAML</span>
                            </a>
                        )}
                    </div>

                    <div className="my-6 flex items-center gap-3 text-[11px] uppercase tracking-widest text-muted-foreground">
                        <span className="h-px flex-1 bg-white/10" />
                        or sign in with a local account
                        <span className="h-px flex-1 bg-white/10" />
                    </div>

                    <form className="space-y-3" onSubmit={handleLocalSignIn}>
                        <Input
                            autoFocus
                            placeholder="Username"
                            value={username}
                            onChange={(e) => setUsername(e.target.value)}
                            className="h-11 rounded-2xl border-white/10 bg-white/[0.04] px-4 backdrop-blur-md"
                        />
                        <Input
                            type="password"
                            placeholder="Password"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            className="h-11 rounded-2xl border-white/10 bg-white/[0.04] px-4 backdrop-blur-md"
                        />
                        <button
                            type="submit"
                            disabled={pending || !username || !password}
                            className="sso-btn group flex w-full items-center justify-center gap-2 rounded-2xl border border-white/10 bg-white/[0.06] px-4 py-3.5 text-sm font-medium backdrop-blur-md transition-colors hover:bg-white/[0.11] disabled:opacity-60"
                        >
                            {pending ? <Loader2 className="h-4 w-4 animate-spin text-i17-cosmic" /> : <KeyRound className="h-4 w-4" />}
                            {pending ? "Signing in…" : "Sign in"}
                        </button>

                        {labHint && (
                            <button
                                type="button"
                                onClick={fillDemo}
                                className="w-full text-center text-[11px] text-muted-foreground underline-offset-2 hover:underline"
                            >
                                Lab mode — fill {labHint}
                            </button>
                        )}
                    </form>

                    <div className="mt-8 flex items-center gap-2 rounded-xl border border-white/5 bg-white/[0.02] px-3 py-2.5 text-[11px] text-muted-foreground">
                        <Lock className="h-3.5 w-3.5 text-i17-sage" />
                        Sessions are scoped per device and expire automatically.
                    </div>
                </div>
            </div>

            <div className="absolute bottom-5 left-0 right-0 text-center text-[11px] text-muted-foreground/60">
                Axiom by Zyvor · zyvor.dev
            </div>
        </div>
    );
}
