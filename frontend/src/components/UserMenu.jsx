import { useNavigate } from "react-router-dom";
import { LogOut, ShieldCheck } from "lucide-react";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/lib/auth";
import { USER_MENU } from "@/constants/testIds";
import { toast } from "sonner";

export default function UserMenu() {
    const { user, signOut } = useAuth();
    const navigate = useNavigate();
    if (!user) return null;

    const handleSignOut = () => {
        signOut();
        toast.info("Signed out", { description: "Your Axiom session was ended." });
        navigate("/login", { replace: true });
    };

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <button
                    data-testid={USER_MENU.trigger}
                    className="grid h-9 w-9 place-items-center rounded-full bg-gradient-to-br from-[#b49bd8] via-[#7ab6dd] to-[#4d80c7] text-xs font-semibold text-white ring-1 ring-white/20 transition-transform hover:scale-105"
                >
                    {user.initials}
                </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="glass w-60 rounded-2xl border-white/10 p-2">
                <DropdownMenuLabel className="px-2 py-1.5">
                    <div className="text-sm font-medium">{user.name}</div>
                    <div className="text-xs font-normal text-muted-foreground">{user.role}</div>
                </DropdownMenuLabel>
                <div className="mx-2 mb-1 flex items-center gap-1.5 rounded-lg bg-white/[0.04] px-2 py-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                    <ShieldCheck className="h-3 w-3 text-[#93c88f]" />
                    {user.authMethod === "oidc" ? "SSO" : "Local"} · {user.mode}
                </div>
                <DropdownMenuSeparator className="bg-white/10" />
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
    );
}
