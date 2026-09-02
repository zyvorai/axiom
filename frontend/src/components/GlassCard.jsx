import { cn } from "@/lib/utils";

/** Flat section wrapper — no card chrome (apple.com-style). */
export function GlassCard({ className, children, ...rest }) {
    return (
        <div className={cn("relative", className)} {...rest}>
            {children}
        </div>
    );
}

export default GlassCard;
