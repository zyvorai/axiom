import { cn } from "@/lib/utils";

// A numbered-step header, e.g. for the Run-workload wizard's Form/Intent/
// Validate tabs or the Getting Started checklist — replaces a plain tab
// strip with an explicit sequence (01, 02, 03...) so a multi-step flow reads
// as a route with an order, not just a set of unordered options.
// steps: [{ key, label }]. `active` is the key of the current step.
export default function Stepper({ steps, active, onSelect, className }) {
    return (
        <div className={cn("flex flex-wrap items-center gap-1", className)}>
            {steps.map((s, i) => {
                const isActive = s.key === active;
                const Comp = onSelect ? "button" : "div";
                return (
                    <Comp
                        key={s.key}
                        type={onSelect ? "button" : undefined}
                        onClick={onSelect ? () => onSelect(s.key) : undefined}
                        aria-current={isActive ? "step" : undefined}
                        className={cn(
                            "flex items-center gap-2 rounded-xl px-3 py-2 text-sm transition-colors",
                            onSelect && "cursor-pointer hover:bg-muted",
                            isActive ? "bg-muted text-foreground" : "text-muted-foreground"
                        )}
                    >
                        <span
                            className={cn(
                                "mono text-[11px] tabular-nums",
                                isActive ? "text-tone-link" : "text-muted-foreground"
                            )}
                        >
                            {String(i + 1).padStart(2, "0")}
                        </span>
                        <span className={isActive ? "font-medium" : undefined}>{s.label}</span>
                    </Comp>
                );
            })}
        </div>
    );
}
