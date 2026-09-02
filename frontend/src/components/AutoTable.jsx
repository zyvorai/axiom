import { formatDistanceToNowStrict } from "date-fns";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Badge } from "@/components/ui/badge";

const HIDDEN_KEYS = new Set(["id", "cluster_id", "pool_id", "backend_native_id", "connection_ref", "secret_ref"]);
const STATUS_KEYS = ["state", "status", "health"];

function pretty(key) {
    return key.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function formatValue(v) {
    if (v === null || v === undefined) return "—";
    if (typeof v === "boolean") return v ? "yes" : "no";
    if (typeof v === "object") return JSON.stringify(v);
    return String(v);
}

// getPath resolves a dot-path against a row, falling back to the
// equivalent metadata.<key> location — CRD instances keep name/namespace
// under metadata, but a plugin manifest may reference them directly.
function getPath(row, path) {
    if (path === "name") return row?.metadata?.name ?? row?.name;
    if (path === "namespace") return row?.metadata?.namespace ?? row?.namespace;
    if (path === "age") return row?.metadata?.creationTimestamp ?? row?.creationTimestamp ?? row?.age;
    return path.split(".").reduce((acc, k) => (acc == null ? undefined : acc[k]), row);
}

// formatAge handles both a raw ISO timestamp (real Kubernetes objects, under
// metadata.creationTimestamp) and an already-human-readable string like "5d"
// (demo data, which ships pre-formatted ages instead of timestamps).
function formatAge(raw) {
    if (!raw) return "—";
    const parsed = Date.parse(raw);
    if (!Number.isNaN(parsed)) return formatDistanceToNowStrict(new Date(parsed), { addSuffix: true });
    return String(raw);
}

function renderColumnValue(row, col) {
    const raw = getPath(row, col.path);
    if (col.type === "age") return formatAge(raw);
    if (col.type === "status") return <StatusBadge status={String(raw ?? "unknown")} />;
    if (col.type === "badge") {
        return (
            <Badge variant="outline" className="text-[10px]">
                {formatValue(raw)}
            </Badge>
        );
    }
    if (col.type === "count") return Array.isArray(raw) ? raw.length : formatValue(raw);
    return formatValue(raw);
}

// AutoTable renders an array of arbitrary/unknown-shape objects as a table.
// By default it guesses columns automatically without any hardcoded field
// names — used for both Atlas (whose REST shapes vary per resource type)
// and the generic CRD browser (whose shapes are, by definition, unknowable
// ahead of time). Passing an explicit `columns` list (as declared by a
// Tier 1 plugin manifest) overrides that guessing with a fixed, ordered
// set of columns instead.
export default function AutoTable({ title, icon: Icon, items, emptyHint, onRowClick, columns, rowActions }) {
    const rows = Array.isArray(items) ? items : null;

    if (columns && columns.length > 0) {
        return (
            <GlassCard className="p-0 overflow-hidden">
                <div className="flex items-center justify-between border-b px-5 py-4">
                    <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                        <Icon className="h-4 w-4 text-i17-cosmic" /> {title}
                    </div>
                    <span className="text-xs text-muted-foreground">{rows ? rows.length : 0}</span>
                </div>
                <div className="max-h-[420px] overflow-auto scrollbar-thin">
                    {(!rows || rows.length === 0) && (
                        <div className="px-5 py-10 text-center text-sm text-muted-foreground">{emptyHint}</div>
                    )}
                    {rows &&
                        rows.map((row, i) => (
                            <div
                                key={getPath(row, "name") ?? i}
                                onClick={onRowClick ? () => onRowClick(row) : undefined}
                                className={`flex flex-wrap items-center gap-x-6 gap-y-1 border-b px-5 py-4 last:border-none transition-colors hover:bg-foreground/[0.03] ${onRowClick ? "cursor-pointer" : ""}`}
                            >
                                {columns.map((col, ci) => (
                                    <div
                                        key={col.path}
                                        className={ci === 0 ? "mono min-w-[160px] flex-1 truncate text-sm" : "text-xs text-muted-foreground"}
                                    >
                                        {ci !== 0 && <span className="opacity-70">{col.label}: </span>}
                                        <span className={ci === 0 ? undefined : "mono text-foreground"}>{renderColumnValue(row, col)}</span>
                                    </div>
                                ))}
                                {rowActions && (
                                    <div className="ml-auto flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                                        {rowActions(row)}
                                    </div>
                                )}
                            </div>
                        ))}
                </div>
            </GlassCard>
        );
    }

    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center justify-between border-b px-5 py-4">
                <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                    <Icon className="h-4 w-4 text-i17-cosmic" /> {title}
                </div>
                <span className="text-xs text-muted-foreground">{rows ? rows.length : 0}</span>
            </div>
            <div className="max-h-[420px] overflow-auto scrollbar-thin">
                {(!rows || rows.length === 0) && (
                    <div className="px-5 py-10 text-center text-sm text-muted-foreground">{emptyHint}</div>
                )}
                {rows &&
                    rows.map((row, i) => {
                        const nameKey = ["name", "bucket", "intent", "id"].find((k) => row[k] !== undefined);
                        const name = nameKey ? row[nameKey] : `#${i}`;
                        const statusKey = STATUS_KEYS.find((k) => row[k] !== undefined);
                        const fields = Object.entries(row).filter(
                            ([k]) => !HIDDEN_KEYS.has(k) && k !== nameKey && k !== statusKey
                        );
                        return (
                            <div
                                key={row.id ?? i}
                                onClick={onRowClick ? () => onRowClick(row) : undefined}
                                className={`flex flex-wrap items-center gap-x-6 gap-y-1 border-b px-5 py-4 last:border-none transition-colors hover:bg-foreground/[0.03] ${onRowClick ? "cursor-pointer" : ""}`}
                            >
                                <div className="mono min-w-[160px] flex-1 truncate text-sm">{String(name)}</div>
                                {fields.slice(0, 5).map(([k, v]) => (
                                    <div key={k} className="text-xs text-muted-foreground">
                                        <span className="opacity-70">{pretty(k)}:</span>{" "}
                                        <span className="mono text-foreground">{formatValue(v)}</span>
                                    </div>
                                ))}
                                {statusKey && <StatusBadge status={String(row[statusKey])} />}
                            </div>
                        );
                    })}
            </div>
        </GlassCard>
    );
}
