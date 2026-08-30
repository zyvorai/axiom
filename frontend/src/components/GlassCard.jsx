import { cn } from "@/lib/utils";

// .glass already declares a transform/box-shadow transition (index.css)
// but is shared by non-card chrome too (Sidebar, dialogs, dropdowns) where
// a hover lift would look wrong, so the actual :hover rule lives here,
// scoped to content cards only, instead of on the shared class globally.
export function GlassCard({ className, children, ...rest }) {
    return (
        <div
            className={cn(
                "glass specular relative overflow-hidden transition-[transform,box-shadow] duration-200 ease-out hover:-translate-y-0.5 hover:shadow-[0_8px_20px_rgba(0,0,0,0.2)]",
                className
            )}
            {...rest}
        >
            {children}
        </div>
    );
}

export default GlassCard;
