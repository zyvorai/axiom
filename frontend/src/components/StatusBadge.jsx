import { cn } from "@/lib/utils";

const MAP = {
    Healthy:  { tone: "var(--green)",  label: "Healthy" },
    Ready:    { tone: "var(--green)",  label: "Ready" },
    Running:  { tone: "var(--green)",  label: "Running" },
    Bound:    { tone: "var(--green)",  label: "Bound" },
    Normal:   { tone: "var(--green)",  label: "Normal" },
    Complete: { tone: "var(--green)",  label: "Complete" },
    Degraded: { tone: "var(--orange)", label: "Degraded" },
    Warning:  { tone: "var(--orange)", label: "Warning" },
    Pending:  { tone: "var(--amber)",  label: "Pending" },
    Paused:   { tone: "var(--amber)",  label: "Paused" },
    Migrating:{ tone: "var(--blue)", label: "Migrating" },
    SchedulingDisabled: { tone: "var(--blue)", label: "Cordoned" },
    NotReady: { tone: "var(--red)",  label: "NotReady" },
    Failed:   { tone: "var(--red)",  label: "Failed" },
    dropped:  { tone: "var(--red)",  label: "dropped" },
    forwarded:{ tone: "var(--green)",   label: "forwarded" },
    audit:    { tone: "var(--blue)", label: "audit" },
};

/** Flat tone-dot pill — see .pill in axiom-shell.css. */
export default function StatusBadge({ status, className }) {
    const s = MAP[status] || { tone: "var(--blue)", label: status };
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {s.label}
        </span>
    );
}
