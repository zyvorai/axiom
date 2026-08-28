import { cn } from "@/lib/utils";

const MAP = {
    Healthy:  { dot: "ok",   label: "Healthy",  color: "text-emerald-500 dark:text-emerald-400" },
    Ready:    { dot: "ok",   label: "Ready",    color: "text-emerald-500 dark:text-emerald-400" },
    Running:  { dot: "ok",   label: "Running",  color: "text-emerald-500 dark:text-emerald-400" },
    Bound:    { dot: "ok",   label: "Bound",    color: "text-emerald-500 dark:text-emerald-400" },
    Normal:   { dot: "ok",   label: "Normal",   color: "text-emerald-500 dark:text-emerald-400" },
    Degraded: { dot: "warn", label: "Degraded", color: "text-amber-500 dark:text-amber-400" },
    Warning:  { dot: "warn", label: "Warning",  color: "text-amber-500 dark:text-amber-400" },
    Pending:  { dot: "warn", label: "Pending",  color: "text-amber-500 dark:text-amber-400" },
    Paused:   { dot: "warn", label: "Paused",   color: "text-amber-500 dark:text-amber-400" },
    Migrating:{ dot: "info", label: "Migrating",color: "text-sky-500 dark:text-sky-400" },
    SchedulingDisabled: { dot: "info", label: "Cordoned", color: "text-sky-500 dark:text-sky-400" },
    NotReady: { dot: "err",  label: "NotReady", color: "text-rose-500 dark:text-rose-400" },
    Failed:   { dot: "err",  label: "Failed",   color: "text-rose-500 dark:text-rose-400" },
    dropped:  { dot: "err",  label: "dropped",  color: "text-rose-500 dark:text-rose-400" },
    forwarded:{ dot: "ok",   label: "forwarded",color: "text-emerald-500 dark:text-emerald-400" },
    audit:    { dot: "info", label: "audit",    color: "text-sky-500 dark:text-sky-400" },
};

export default function StatusBadge({ status, className }) {
    const s = MAP[status] || { dot: "info", label: status, color: "text-sky-500 dark:text-sky-400" };
    return (
        <span className={cn("inline-flex items-center gap-2 text-xs font-medium", s.color, className)}>
            <span className={cn("status-dot", s.dot)} />
            {s.label}
        </span>
    );
}
