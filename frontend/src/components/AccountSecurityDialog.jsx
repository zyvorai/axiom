import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Copy, Download, KeyRound, LogOut, ShieldCheck, ShieldOff } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { MFA } from "@/constants/testIds";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";

function copy(text, what) {
    navigator.clipboard
        ?.writeText(text)
        .then(() => toast.success(`${what} copied`))
        .catch(() => toast.error("Could not copy — select the text and copy it manually"));
}

// Two-factor sign-in (TOTP) and sign-out-everywhere for the signed-in person's
// own account. Accounts managed by an identity provider are pointed there.
export default function AccountSecurityDialog({ open, onOpenChange }) {
    const { signOut } = useAuth();
    const navigate = useNavigate();
    const [status, setStatus] = useState(null); // {available, enabled, recoveryCodes, managed}
    const [loadError, setLoadError] = useState("");
    const [phase, setPhase] = useState("idle"); // idle | enrol | recovery
    const [password, setPassword] = useState("");
    const [setup, setSetup] = useState(null); // {secret, uri}
    const [code, setCode] = useState("");
    const [recovery, setRecovery] = useState([]);
    const [disCode, setDisCode] = useState("");
    const [disPassword, setDisPassword] = useState("");
    const [busy, setBusy] = useState(false);

    const reset = useCallback(() => {
        setPhase("idle");
        setPassword("");
        setSetup(null);
        setCode("");
        setRecovery([]);
        setDisCode("");
        setDisPassword("");
    }, []);

    const load = useCallback(async () => {
        setLoadError("");
        try {
            const { data } = await api.get("/auth/mfa");
            setStatus(data);
        } catch (e) {
            setLoadError(errText(e));
        }
    }, []);

    useEffect(() => {
        if (open) {
            reset();
            load();
        }
    }, [open, load, reset]);

    // Anything that revoked this account's sessions ends here: go back to sign-in.
    const leave = useCallback(
        async (message) => {
            await signOut();
            onOpenChange(false);
            toast.info(message);
            navigate("/login", { replace: true });
        },
        [signOut, onOpenChange, navigate]
    );

    const act = async (fn) => {
        setBusy(true);
        try {
            await fn();
        } catch (e) {
            toast.error("Failed", { description: errText(e) });
        } finally {
            setBusy(false);
        }
    };

    const startSetup = () =>
        act(async () => {
            const { data } = await api.post("/auth/mfa/setup", { password });
            setSetup(data);
            setPassword("");
            setPhase("enrol");
        });

    const confirmEnable = () =>
        act(async () => {
            const { data } = await api.post("/auth/mfa/enable", { code });
            setRecovery(data.recoveryCodes || []);
            setPhase("recovery");
        });

    const disable = () =>
        act(async () => {
            await api.post("/auth/mfa/disable", { password: disPassword, code: disCode });
            await leave("Two-factor sign-in turned off. Sign in again.");
        });

    const signOutEverywhere = () =>
        act(async () => {
            await api.post("/auth/logout-all");
            await leave("Signed out of every device.");
        });

    const downloadCodes = () => {
        const blob = new Blob([`Axiom recovery codes\nEach code can be used once.\n\n${recovery.join("\n")}\n`], { type: "text/plain" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "axiom-recovery-codes.txt";
        a.click();
        URL.revokeObjectURL(url);
    };

    return (
        <Dialog open={open} onOpenChange={(o) => phase === "recovery" ? null : onOpenChange(o)}>
            <DialogContent className="max-w-md" data-testid={MFA.dialog}>
                <DialogHeader>
                    <DialogTitle>Account security</DialogTitle>
                    <DialogDescription>Two-factor sign-in and active sessions for your account.</DialogDescription>
                </DialogHeader>

                {loadError ? <p className="text-sm text-tone-red">{loadError}</p> : null}
                {!status && !loadError ? <p className="text-sm text-muted-foreground">Loading…</p> : null}

                {status && !status.available ? (
                    <div className="space-y-2" data-testid={MFA.status}>
                        <p className="flex items-center gap-2 text-sm">
                            <ShieldCheck className="h-4 w-4 text-tone-green" aria-hidden="true" />
                            {status.enabled ? "Two-factor sign-in is on." : "Two-factor sign-in is off."}
                        </p>
                        <p className="text-sm text-muted-foreground">
                            This account&apos;s second factor is managed by {status.managed}.
                        </p>
                    </div>
                ) : null}

                {status && status.available && phase === "idle" && !status.enabled ? (
                    <form
                        className="space-y-3"
                        onSubmit={(e) => {
                            e.preventDefault();
                            if (password) startSetup();
                        }}
                    >
                        <p className="flex items-center gap-2 text-sm" data-testid={MFA.status}>
                            <ShieldOff className="h-4 w-4 text-tone-amber" aria-hidden="true" />
                            Two-factor sign-in is off.
                        </p>
                        <p className="text-sm text-muted-foreground">
                            Add a one-time code from an authenticator app to your password. Confirm your password to start.
                        </p>
                        <div className="space-y-1.5">
                            <Label htmlFor="mfa-setup-password" className="text-xs text-muted-foreground">Password</Label>
                            <Input
                                id="mfa-setup-password"
                                data-testid={MFA.setupPassword}
                                type="password"
                                autoComplete="current-password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                className="bg-muted/30"
                            />
                        </div>
                        <button type="submit" className="btn btn-fill" data-testid={MFA.setupStart} disabled={busy || !password}>
                            <KeyRound size={14} /> {busy ? "Starting…" : "Set up two-factor"}
                        </button>
                    </form>
                ) : null}

                {phase === "enrol" && setup ? (
                    <form
                        className="space-y-3"
                        onSubmit={(e) => {
                            e.preventDefault();
                            if (code) confirmEnable();
                        }}
                    >
                        <p className="text-sm text-muted-foreground">
                            In your authenticator app choose <b>Add account</b> → <b>Enter a setup key</b>, and type this key (time-based, 6 digits):
                        </p>
                        <div className="flex items-center gap-2 rounded-xl bg-muted px-3 py-2">
                            <code className="mono flex-1 break-all text-sm" data-testid={MFA.secret}>{setup.secret}</code>
                            <button
                                type="button"
                                className="btn btn-line btn-sm"
                                aria-label="Copy setup key"
                                data-testid={MFA.copySecret}
                                onClick={() => copy(setup.secret.replace(/ /g, ""), "Setup key")}
                            >
                                <Copy size={13} />
                            </button>
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="mfa-enable-code" className="text-xs text-muted-foreground">Code from the app</Label>
                            <Input
                                id="mfa-enable-code"
                                data-testid={MFA.enableCode}
                                inputMode="numeric"
                                autoComplete="one-time-code"
                                placeholder="123456"
                                value={code}
                                onChange={(e) => setCode(e.target.value)}
                                className="bg-muted/30"
                            />
                        </div>
                        <button type="submit" className="btn btn-fill" data-testid={MFA.enableSubmit} disabled={busy || !code}>
                            {busy ? "Checking…" : "Turn on two-factor"}
                        </button>
                    </form>
                ) : null}

                {phase === "recovery" ? (
                    <div className="space-y-3">
                        <p className="text-sm">
                            Two-factor sign-in is on. Save these recovery codes now — they are shown <b>only once</b>, and each works a single time if you lose your device.
                        </p>
                        <ul className="grid grid-cols-2 gap-1.5 rounded-xl bg-muted p-3" data-testid={MFA.recoveryCodes}>
                            {recovery.map((c) => (
                                <li key={c} className="mono text-sm">{c}</li>
                            ))}
                        </ul>
                        <div className="flex flex-wrap gap-2">
                            <button type="button" className="btn btn-line btn-sm" onClick={() => copy(recovery.join("\n"), "Recovery codes")}>
                                <Copy size={13} /> Copy
                            </button>
                            <button type="button" className="btn btn-line btn-sm" onClick={downloadCodes}>
                                <Download size={13} /> Download
                            </button>
                        </div>
                        <p className="text-xs text-muted-foreground">Your sessions were ended for safety; you will sign in again with a code.</p>
                        <button type="button" className="btn btn-fill" data-testid={MFA.recoveryDone} onClick={() => leave("Two-factor sign-in is on. Sign in again.")}>
                            I&apos;ve saved them
                        </button>
                    </div>
                ) : null}

                {status && status.available && phase === "idle" && status.enabled ? (
                    <form
                        className="space-y-3"
                        onSubmit={(e) => {
                            e.preventDefault();
                            if (disPassword && disCode) disable();
                        }}
                    >
                        <p className="flex items-center gap-2 text-sm" data-testid={MFA.status}>
                            <ShieldCheck className="h-4 w-4 text-tone-green" aria-hidden="true" />
                            Two-factor sign-in is on · {status.recoveryCodes} recovery {status.recoveryCodes === 1 ? "code" : "codes"} left
                        </p>
                        <p className="text-sm text-muted-foreground">To turn it off, confirm your password and a current code (or a recovery code).</p>
                        <div className="grid grid-cols-2 gap-2">
                            <div className="space-y-1.5">
                                <Label htmlFor="mfa-disable-password" className="text-xs text-muted-foreground">Password</Label>
                                <Input
                                    id="mfa-disable-password"
                                    data-testid={MFA.disablePassword}
                                    type="password"
                                    autoComplete="current-password"
                                    value={disPassword}
                                    onChange={(e) => setDisPassword(e.target.value)}
                                    className="bg-muted/30"
                                />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="mfa-disable-code" className="text-xs text-muted-foreground">Code</Label>
                                <Input
                                    id="mfa-disable-code"
                                    data-testid={MFA.disableCode}
                                    autoComplete="one-time-code"
                                    value={disCode}
                                    onChange={(e) => setDisCode(e.target.value)}
                                    className="bg-muted/30"
                                />
                            </div>
                        </div>
                        <button type="submit" className="btn btn-danger" data-testid={MFA.disableSubmit} disabled={busy || !disPassword || !disCode}>
                            Turn off two-factor
                        </button>
                    </form>
                ) : null}

                {phase !== "recovery" ? (
                    <div className="mt-2 space-y-2 border-t border-border pt-3">
                        <p className="text-sm text-muted-foreground">
                            Signed in somewhere you no longer trust? End every session for this account on every device.
                        </p>
                        <button type="button" className="btn btn-line btn-sm" data-testid={MFA.signOutAll} onClick={signOutEverywhere} disabled={busy}>
                            <LogOut size={13} /> Sign out everywhere
                        </button>
                    </div>
                ) : null}
            </DialogContent>
        </Dialog>
    );
}
