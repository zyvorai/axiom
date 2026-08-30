import { cn } from "@/lib/utils";

// .glass already declares a transform/box-shadow transition (index.css)
// but is shared by non-card chrome too (Sidebar, dialogs, dropdowns) where
// a hover lift would look wrong, so the actual :hover rule lives here,
// scoped to content cards only, instead of on the shared class globally.
export function GlassCard({ className, children, ...rest }) {
    return (
        <div
            className={cn(
                "glass specular relative overflow-hidden hover:-translate-y-0.5 hover:shadow-[0_45px_90px_-30px_rgb(0_0_0_/_0.7)]",
                className
            )}
            {...rest}
        >
            {children}
        </div>
    );
}

export default GlassCard;
