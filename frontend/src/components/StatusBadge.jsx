import { cn } from "@/lib/utils";

const MAP = {
    Healthy:  { tone: "var(--green, hsl(var(--k-ok)))",  label: "Healthy" },
    Ready:    { tone: "var(--green, hsl(var(--k-ok)))",  label: "Ready" },
    Running:  { tone: "var(--green, hsl(var(--k-ok)))",  label: "Running" },
    Bound:    { tone: "var(--green, hsl(var(--k-ok)))",  label: "Bound" },
    Normal:   { tone: "var(--green, hsl(var(--k-ok)))",  label: "Normal" },
    Complete: { tone: "var(--green, hsl(var(--k-ok)))",  label: "Complete" },
    Degraded: { tone: "var(--orange, hsl(var(--k-warn)))", label: "Degraded" },
    Warning:  { tone: "var(--orange, hsl(var(--k-warn)))", label: "Warning" },
    Pending:  { tone: "var(--amber, hsl(var(--k-warn)))",  label: "Pending" },
    Paused:   { tone: "var(--amber, hsl(var(--k-warn)))",  label: "Paused" },
    Migrating:{ tone: "var(--blue, hsl(var(--k-info)))", label: "Migrating" },
    SchedulingDisabled: { tone: "var(--blue, hsl(var(--k-info)))", label: "Cordoned" },
    NotReady: { tone: "var(--red, hsl(var(--k-err)))",  label: "NotReady" },
    Failed:   { tone: "var(--red, hsl(var(--k-err)))",  label: "Failed" },
    dropped:  { tone: "var(--red, hsl(var(--k-err)))",  label: "dropped" },
    forwarded:{ tone: "var(--green, hsl(var(--k-ok)))",   label: "forwarded" },
    audit:    { tone: "var(--blue, hsl(var(--k-info)))", label: "audit" },
};

/** Flat tone-dot pill — see .pill in axiom-shell.css. */
export default function StatusBadge({ status, className }) {
    const s = MAP[status] || { tone: "var(--blue, hsl(var(--k-info)))", label: status };
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {s.label}
        </span>
    );
}
