import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

const TONES = {
    critical: { tone: "var(--red)", labelKey: "pages:severity.critical" },
    warning: { tone: "var(--orange)", labelKey: "pages:severity.warning" },
    info: { tone: "var(--blue)", labelKey: "pages:severity.info" },
};

const SOURCE_TONES = {
    event: { tone: "var(--teal)", labelKey: "pages:severity.source.event" },
    audit: { tone: "var(--purple)", labelKey: "pages:severity.source.audit" },
    alert: { tone: "var(--orange)", labelKey: "pages:severity.source.alert" },
    rollout: { tone: "var(--green)", labelKey: "pages:severity.source.rollout" },
};

/** Severity pill (critical / warning / info) for health findings and timeline rows. */
export function SeverityBadge({ severity, className }) {
    const { t } = useTranslation();
    const s = TONES[severity] || TONES.info;
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {t(s.labelKey)}
        </span>
    );
}

const STATUS_TONES = {
    critical: { tone: "var(--red)", labelKey: "pages:severity.status.critical" },
    warning: { tone: "var(--orange)", labelKey: "pages:severity.status.warning" },
    healthy: { tone: "var(--green)", labelKey: "pages:severity.status.healthy" },
};

/** Overall health-check status pill (healthy / warning / critical). */
export function HealthStatusBadge({ status, className }) {
    const { t } = useTranslation();
    const s = STATUS_TONES[status] || STATUS_TONES.healthy;
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {t(s.labelKey)}
        </span>
    );
}

/** Source pill (event / audit / alert / rollout) for timeline rows. */
export function SourceBadge({ source, className }) {
    const { t } = useTranslation();
    const s = SOURCE_TONES[source] || { tone: "var(--ink-3)", label: source };
    return (
        <span className={cn("pill", className)} style={{ "--c": s.tone }}>
            <i aria-hidden="true" />
            {s.labelKey ? t(s.labelKey) : s.label}
        </span>
    );
}

export default SeverityBadge;
