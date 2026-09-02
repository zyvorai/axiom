import { Eye } from "lucide-react";
import { useAuth } from "@/lib/auth";

export default function ViewerBanner() {
    const { isViewer } = useAuth();
    if (!isViewer) return null;

    return (
        <div className="glass-inset flex items-center gap-2 rounded-2xl px-3.5 py-2 text-xs text-muted-foreground">
            <Eye className="h-3.5 w-3.5 shrink-0 text-i17-sky" />
            You're signed in as a <span className="font-medium text-foreground">Viewer</span> — read-only access. Every create, edit, and delete action is disabled server-side.
        </div>
    );
}
