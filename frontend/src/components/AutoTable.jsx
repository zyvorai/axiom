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
function TableChrome({ title, Icon, count }) {
    return (
        <div className="card-h">
            <h2 style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 15 }}>
                <Icon className="h-4 w-4" style={{ color: "var(--blue)" }} /> {title}
            </h2>
            <span className="r" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>{count}</span>
        </div>
    );
}

export default function AutoTable({ title, icon: Icon, items, emptyHint, onRowClick, columns, rowActions }) {
    const rows = Array.isArray(items) ? items : null;

    if (columns && columns.length > 0) {
        return (
            <GlassCard className="overflow-hidden">
                <TableChrome title={title} Icon={Icon} count={rows ? rows.length : 0} />
                {(!rows || rows.length === 0) ? (
                    <div className="empty">{emptyHint}</div>
                ) : (
                    <div style={{ overflowX: "auto" }}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    {columns.map((col) => (
                                        <th key={col.path}>{col.label}</th>
                                    ))}
                                    {rowActions && <th />}
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((row, i) => (
                                    <tr
                                        key={getPath(row, "name") ?? i}
                                        onClick={onRowClick ? () => onRowClick(row) : undefined}
                                        style={onRowClick ? { cursor: "pointer" } : undefined}
                                    >
                                        {columns.map((col, ci) => (
                                            <td key={col.path} className={ci === 0 ? "name mono" : "num"}>
                                                {renderColumnValue(row, col)}
                                            </td>
                                        ))}
                                        {rowActions && (
                                            <td onClick={(e) => e.stopPropagation()}>{rowActions(row)}</td>
                                        )}
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </GlassCard>
        );
    }

    return (
        <GlassCard className="overflow-hidden">
            <TableChrome title={title} Icon={Icon} count={rows ? rows.length : 0} />
            {(!rows || rows.length === 0) && <div className="empty">{emptyHint}</div>}
            <div className="stack">
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
                                style={{
                                    display: "flex", flexWrap: "wrap", alignItems: "center", gap: "4px 24px",
                                    borderBottom: "1px solid var(--line)", paddingBottom: 12, cursor: onRowClick ? "pointer" : undefined,
                                }}
                            >
                                <div className="mono" style={{ minWidth: 160, flex: 1, fontSize: 13.5 }}>{String(name)}</div>
                                {fields.slice(0, 5).map(([k, v]) => (
                                    <div key={k} style={{ fontSize: 12, color: "var(--ink-3)" }}>
                                        {pretty(k)}: <span className="mono" style={{ color: "var(--ink)" }}>{formatValue(v)}</span>
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
