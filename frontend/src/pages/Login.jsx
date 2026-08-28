import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Lock, ShieldCheck, Globe2, Cpu, KeyRound } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { Input } from "@/components/ui/input";
import { SSO } from "@/constants/testIds";
import { toast } from "sonner";

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
                        <div className="z-mark grid h-12 w-12 place-items-center rounded-2xl bg-gradient-to-br from-[#ff7550] via-[#f26b3a] to-[#c8452a] shadow-xl shadow-[#f26b3a]/40 ring-1 ring-white/15">
                            <svg width="24" height="24" viewBox="0 0 18 18" aria-hidden="true">
                                <path d="M2 2h14L6.6 16H16" fill="none" stroke="white" strokeWidth="2.6" strokeLinejoin="round" strokeLinecap="round" />
                            </svg>
                        </div>
                        <span className="text-lg font-semibold tracking-tight lowercase">zyvor</span>
                    </div>

                    <h1 className="fade-up mt-10 text-5xl font-semibold leading-[1.05] tracking-tight lg:text-6xl" style={{ animationDelay: "90ms" }}>
                        Axiom.
                        <br />
                        <span className="bg-gradient-to-r from-[#ff7550] via-[#e0c996] to-[#6ab6f0] bg-clip-text text-transparent">
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
                                <Icon className="h-4 w-4 text-[#ff7550]" />
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
                        <div className="grid h-10 w-10 place-items-center rounded-xl bg-gradient-to-br from-[#ff7550] to-[#c8452a] ring-1 ring-white/15">
                            <svg width="20" height="20" viewBox="0 0 18 18" aria-hidden="true">
                                <path d="M2 2h14L6.6 16H16" fill="none" stroke="white" strokeWidth="2.6" strokeLinejoin="round" strokeLinecap="round" />
                            </svg>
                        </div>
                        <span className="text-base font-semibold lowercase">zyvor · Axiom</span>
                    </div>

                    <div className="text-[11px] uppercase tracking-[0.24em] text-[#ff7550]/90">Axiom Console</div>
                    <h2 className="mt-2 text-2xl font-semibold tracking-tight">Sign in to continue</h2>
                    <p className="mt-1.5 text-sm text-muted-foreground">
                        {providers ? "Sign in with your Axiom credentials." : "Connecting to Axiom…"}
                    </p>

                    <form className="mt-8 space-y-3" onSubmit={handleLocalSignIn}>
                        <Input
                            data-testid={SSO.google}
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
                            {pending ? <Loader2 className="h-4 w-4 animate-spin text-[#ff7550]" /> : <KeyRound className="h-4 w-4" />}
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

                    {(oidcLoginUrl || samlLoginUrl) && (
                        <div className="mt-6 space-y-2 border-t border-white/5 pt-6">
                            {oidcLoginUrl && (
                                <a
                                    href={oidcLoginUrl}
                                    data-testid={SSO.github}
                                    className="flex w-full items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3 text-sm font-medium backdrop-blur-md transition-colors hover:bg-white/[0.09]"
                                >
                                    <ShieldCheck className="h-4 w-4 text-[#6ab6f0]" />
                                    Continue with OIDC
                                </a>
                            )}
                            {samlLoginUrl && (
                                <a
                                    href={samlLoginUrl}
                                    data-testid={SSO.okta}
                                    className="flex w-full items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3 text-sm font-medium backdrop-blur-md transition-colors hover:bg-white/[0.09]"
                                >
                                    <ShieldCheck className="h-4 w-4 text-[#93c88f]" />
                                    Continue with SAML
                                </a>
                            )}
                        </div>
                    )}

                    <div className="mt-8 flex items-center gap-2 rounded-xl border border-white/5 bg-white/[0.02] px-3 py-2.5 text-[11px] text-muted-foreground">
                        <Lock className="h-3.5 w-3.5 text-[#93c88f]" />
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
