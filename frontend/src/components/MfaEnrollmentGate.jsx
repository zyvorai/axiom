import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { KeyRound, LogOut, ShieldAlert } from "lucide-react";
import { useAuth } from "@/lib/auth";
import AccountSecurityDialog from "@/components/AccountSecurityDialog";
import GlassCard from "@/components/GlassCard";
import { MFA } from "@/constants/testIds";

/**
 * Shown instead of the app when the administrator requires two-factor sign-in
 * for this account's role (AXIOM_REQUIRE_MFA) and it has not been set up yet.
 * The API refuses everything else until it is, so there is nothing useful to
 * render behind it.
 */
export default function MfaEnrollmentGate() {
    const { user, signOut } = useAuth();
    const navigate = useNavigate();
    const [open, setOpen] = useState(true);

    const leave = async () => {
        await signOut();
        navigate("/login", { replace: true });
    };

    return (
        <div className="page" data-testid={MFA.gate}>
            <GlassCard className="mx-auto mt-10 max-w-lg space-y-4 p-6">
                <div className="flex items-center gap-2">
                    <ShieldAlert size={20} className="text-tone-amber" aria-hidden="true" />
                    <h1 className="text-lg font-semibold">Set up two-factor sign-in</h1>
                </div>
                <p className="text-sm text-muted-foreground">
                    Your administrator requires two-factor sign-in for {user?.role ? `the ${user.role} role` : "your role"}. Add an authenticator app to
                    continue — it takes a minute, and you will sign in again with a code afterwards.
                </p>
                <div className="flex flex-wrap gap-2">
                    <button type="button" className="btn btn-fill" data-testid={MFA.gateOpen} onClick={() => setOpen(true)}>
                        <KeyRound size={14} /> Set up now
                    </button>
                    <button type="button" className="btn btn-line" data-testid={MFA.gateSignOut} onClick={leave}>
                        <LogOut size={14} /> Sign out
                    </button>
                </div>
            </GlassCard>
            <AccountSecurityDialog open={open} onOpenChange={setOpen} />
        </div>
    );
}
