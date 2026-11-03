import { cn } from "@/lib/utils";

const TONES = {
    critical: { tone: "var(--red)", label: "Critical" },
    warning: { tone: "var(--orange)", label: "Warning" },
    info: { tone: "var(--blue)", label: "Info" },
};

const SOURCE_TONES = {
    event: { tone: "var(--teal)", label: "Event" },
    audit: { tone: "var(--purple)", label: "Audit" },
    alert: { tone: "var(--orange)", label: "Alert" },
    rollout: { tone: "var(--green)", label: "Rollout" },
};

/** Severity pill (critical / warning / info) for health findings and timeline rows. */
export function SeverityBadge({ severity, className }) {
    const s = TONES[severity] || TONES.info;
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {s.label}
        </span>
    );
}

const STATUS_TONES = {
    critical: { tone: "var(--red)", label: "Critical" },
    warning: { tone: "var(--orange)", label: "Warning" },
    healthy: { tone: "var(--green)", label: "Healthy" },
};

/** Overall health-check status pill (healthy / warning / critical). */
export function HealthStatusBadge({ status, className }) {
    const s = STATUS_TONES[status] || STATUS_TONES.healthy;
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {s.label}
        </span>
    );
}

/** Source pill (event / audit / alert / rollout) for timeline rows. */
export function SourceBadge({ source, className }) {
    const s = SOURCE_TONES[source] || { tone: "var(--ink-3)", label: source };
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {s.label}
        </span>
    );
}

export default SeverityBadge;
