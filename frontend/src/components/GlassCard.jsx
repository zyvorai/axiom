import { cn } from "@/lib/utils";

export function GlassCard({ className, children, ...rest }) {
    return (
        <div className={cn("glass specular relative overflow-hidden", className)} {...rest}>
            {children}
        </div>
    );
}

export default GlassCard;
