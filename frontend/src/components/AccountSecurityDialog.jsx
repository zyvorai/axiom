import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Copy, Download, KeyRound, LogOut, ShieldCheck, ShieldOff } from "lucide-react";
import { toast } from "sonner";
import { Trans, useTranslation } from "react-i18next";
import i18n from "@/lib/i18n";
import QRCode from "qrcode";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { MFA } from "@/constants/testIds";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:generic");

function copy(text, copiedMessage) {
    navigator.clipboard
        ?.writeText(text)
        .then(() => toast.success(copiedMessage))
        .catch(() => toast.error(i18n.t("auth:security.copyFailed")));
}

// Two-factor sign-in (TOTP) and sign-out-everywhere for the signed-in person's
// own account. Accounts managed by an identity provider are pointed there.
export default function AccountSecurityDialog({ open, onOpenChange }) {
    const { signOut } = useAuth();
    const navigate = useNavigate();
    const { t } = useTranslation();
    const [status, setStatus] = useState(null); // {available, enabled, recoveryCodes, managed}
    const [loadError, setLoadError] = useState("");
    const [phase, setPhase] = useState("idle"); // idle | enrol | recovery
    const [password, setPassword] = useState("");
    const [setup, setSetup] = useState(null); // {secret, uri}
    const [code, setCode] = useState("");
    const [recovery, setRecovery] = useState([]);
    const [qr, setQr] = useState("");
    const [disCode, setDisCode] = useState("");
    const [disPassword, setDisPassword] = useState("");
    const [busy, setBusy] = useState(false);

    // Render the otpauth:// URI as a QR code in the browser; the secret never
    // leaves the page (no image service involved).
    useEffect(() => {
        let alive = true;
        setQr("");
        if (setup?.uri) {
            QRCode.toDataURL(setup.uri, { margin: 1, width: 176, errorCorrectionLevel: "M" })
                .then((url) => alive && setQr(url))
                .catch(() => alive && setQr(""));
        }
        return () => {
            alive = false;
        };
    }, [setup]);

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
            toast.error(t("auth:security.failed"), { description: errText(e) });
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
            await leave(t("auth:security.turnedOff"));
        });

    const signOutEverywhere = () =>
        act(async () => {
            await api.post("/auth/logout-all");
            await leave(t("auth:security.signedOutAll"));
        });

    const downloadCodes = () => {
        const blob = new Blob([`${t("auth:security.codesFileHeader")}\n${t("auth:security.codesFileNote")}\n\n${recovery.join("\n")}\n`], { type: "text/plain" });
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
                    <DialogTitle>{t("auth:security.title")}</DialogTitle>
                    <DialogDescription>{t("auth:security.description")}</DialogDescription>
                </DialogHeader>

                {loadError ? <p className="text-sm text-tone-red">{loadError}</p> : null}
                {!status && !loadError ? <p className="text-sm text-muted-foreground">{t("common:states.loading")}</p> : null}

                {status && !status.available ? (
                    <div className="space-y-2" data-testid={MFA.status}>
                        <p className="flex items-center gap-2 text-sm">
                            <ShieldCheck className="h-4 w-4 text-tone-green" aria-hidden="true" />
                            {status.enabled ? t("auth:security.isOn") : t("auth:security.isOff")}
                        </p>
                        <p className="text-sm text-muted-foreground">
                            {t("auth:security.managedBy", { managed: status.managed })}
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
                            {t("auth:security.isOff")}
                        </p>
                        <p className="text-sm text-muted-foreground">
                            {t("auth:security.setupIntro")}
                        </p>
                        <div className="space-y-1.5">
                            <Label htmlFor="mfa-setup-password" className="text-xs text-muted-foreground">{t("auth:login.password")}</Label>
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
                            <KeyRound size={14} /> {busy ? t("auth:security.starting") : t("auth:security.setUp")}
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
                        {qr ? (
                            <div className="flex flex-col items-center gap-1.5">
                                <img src={qr} width={176} height={176} alt={t("auth:security.qrAlt")} data-testid={MFA.qr} className="rounded-lg" />
                                <p className="text-xs text-muted-foreground">{t("auth:security.scan")}</p>
                            </div>
                        ) : null}
                        <p className="text-sm text-muted-foreground">
                            <Trans i18nKey={qr ? "auth:security.manualKeyQr" : "auth:security.manualKey"} components={{ b: <b /> }} />
                        </p>
                        <div className="flex items-center gap-2 rounded-xl bg-muted px-3 py-2">
                            <code className="mono flex-1 break-all text-sm" data-testid={MFA.secret}>{setup.secret}</code>
                            <button
                                type="button"
                                className="btn btn-line btn-sm"
                                aria-label={t("auth:security.copySetupKey")}
                                data-testid={MFA.copySecret}
                                onClick={() => copy(setup.secret.replace(/ /g, ""), t("auth:security.setupKeyCopied"))}
                            >
                                <Copy size={13} />
                            </button>
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="mfa-enable-code" className="text-xs text-muted-foreground">{t("auth:security.codeFromApp")}</Label>
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
                            {busy ? t("auth:security.checking") : t("auth:security.turnOn")}
                        </button>
                    </form>
                ) : null}

                {phase === "recovery" ? (
                    <div className="space-y-3">
                        <p className="text-sm">
                            <Trans i18nKey="auth:security.saveCodes" components={{ b: <b /> }} />
                        </p>
                        <ul className="grid grid-cols-2 gap-1.5 rounded-xl bg-muted p-3" data-testid={MFA.recoveryCodes}>
                            {recovery.map((c) => (
                                <li key={c} className="mono text-sm">{c}</li>
                            ))}
                        </ul>
                        <div className="flex flex-wrap gap-2">
                            <button type="button" className="btn btn-line btn-sm" onClick={() => copy(recovery.join("\n"), t("auth:security.recoveryCodesCopied"))}>
                                <Copy size={13} /> {t("common:actions.copy")}
                            </button>
                            <button type="button" className="btn btn-line btn-sm" onClick={downloadCodes}>
                                <Download size={13} /> {t("common:actions.download")}
                            </button>
                        </div>
                        <p className="text-xs text-muted-foreground">{t("auth:security.sessionsEnded")}</p>
                        <button type="button" className="btn btn-fill" data-testid={MFA.recoveryDone} onClick={() => leave(t("auth:security.onSignInAgain"))}>
                            {t("auth:security.saved")}
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
                            {t("auth:security.onWithCodes", { count: status.recoveryCodes })}
                        </p>
                        <p className="text-sm text-muted-foreground">{t("auth:security.turnOffIntro")}</p>
                        <div className="grid grid-cols-2 gap-2">
                            <div className="space-y-1.5">
                                <Label htmlFor="mfa-disable-password" className="text-xs text-muted-foreground">{t("auth:login.password")}</Label>
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
                                <Label htmlFor="mfa-disable-code" className="text-xs text-muted-foreground">{t("auth:security.code")}</Label>
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
                            {t("auth:security.turnOff")}
                        </button>
                    </form>
                ) : null}

                {phase !== "recovery" ? (
                    <div className="mt-2 space-y-2 border-t border-border pt-3">
                        <p className="text-sm text-muted-foreground">
                            {t("auth:security.signOutAllIntro")}
                        </p>
                        <button type="button" className="btn btn-line btn-sm" data-testid={MFA.signOutAll} onClick={signOutEverywhere} disabled={busy}>
                            <LogOut size={13} /> {t("auth:security.signOutAll")}
                        </button>
                    </div>
                ) : null}
            </DialogContent>
        </Dialog>
    );
}
