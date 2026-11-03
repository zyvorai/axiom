import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { KeyRound, LogOut, ShieldCheck } from "lucide-react";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/lib/auth";
import { USER_MENU, MFA } from "@/constants/testIds";
import AccountSecurityDialog from "@/components/AccountSecurityDialog";
import { toast } from "sonner";

export default function UserMenu() {
    const { user, signOut } = useAuth();
    const navigate = useNavigate();
    const [securityOpen, setSecurityOpen] = useState(false);
    if (!user) return null;

    const handleSignOut = () => {
        signOut();
        toast.info("Signed out", { description: "Your Axiom session was ended." });
        navigate("/login", { replace: true });
    };

    return (
        <>
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <button
                    data-testid={USER_MENU.trigger}
                    className="grid h-8 w-8 place-items-center rounded-full bg-primary text-[11px] font-semibold text-primary-foreground transition-transform hover:scale-105"
                >
                    {user.initials}
                </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-60 rounded-2xl p-2">
                <DropdownMenuLabel className="px-2 py-1.5">
                    <div className="text-sm font-medium">{user.name}</div>
                    <div className="text-xs font-normal text-muted-foreground">{user.role}</div>
                </DropdownMenuLabel>
                <div className="mx-2 mb-1 flex items-center gap-1.5 rounded-lg bg-muted px-2 py-1 text-[11px] uppercase tracking-widest text-muted-foreground">
                    <ShieldCheck className="h-3 w-3 text-tone-green" />
                    {user.authMethod === "oidc" ? "SSO" : "Local"} · {user.mode}
                </div>
                <DropdownMenuSeparator className="bg-muted" />
                <DropdownMenuItem
                    data-testid={MFA.menuItem}
                    onClick={() => setSecurityOpen(true)}
                    className="cursor-pointer rounded-xl text-sm"
                >
                    <KeyRound className="mr-2 h-4 w-4" />
                    Account security
                </DropdownMenuItem>
                <DropdownMenuItem
                    data-testid={USER_MENU.signOut}
                    onClick={handleSignOut}
                    className="cursor-pointer rounded-xl text-sm"
                >
                    <LogOut className="mr-2 h-4 w-4" />
                    Sign out
                </DropdownMenuItem>
            </DropdownMenuContent>
        </DropdownMenu>
        <AccountSecurityDialog open={securityOpen} onOpenChange={setSecurityOpen} />
        </>
    );
}
