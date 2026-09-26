import { cn } from "@/lib/utils";

/** Netra card surface (1px border, 28px radius, blue top hairline) — see .card in axiom-shell.css. */
export function GlassCard({ className, children, ...rest }) {
    return (
        <div className={cn("card", className)} {...rest}>
            {children}
        </div>
    );
}

export default GlassCard;
