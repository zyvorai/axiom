import { cn } from "@/lib/utils";

/** Flat apple.com-dark card surface — see .card in axiom-shell.css. */
export function GlassCard({ className, children, ...rest }) {
    return (
        <div className={cn("card", className)} {...rest}>
            {children}
        </div>
    );
}

export default GlassCard;
