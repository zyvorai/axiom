import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";

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

// AutoTable renders an array of arbitrary/unknown-shape objects as a table
// without any hardcoded field names — used for both Atlas (whose REST shapes
// vary per resource type) and the generic CRD browser (whose shapes are, by
// definition, unknowable ahead of time).
export default function AutoTable({ title, icon: Icon, items, emptyHint, onRowClick }) {
    const rows = Array.isArray(items) ? items : null;
    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-[#ff7550]" /> {title}
                </div>
                <span className="text-xs text-muted-foreground">{rows ? rows.length : 0}</span>
            </div>
            <div className="max-h-[420px] overflow-auto scrollbar-thin">
                {(!rows || rows.length === 0) && (
                    <div className="px-4 py-8 text-center text-sm text-muted-foreground">{emptyHint}</div>
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
                                className={`flex flex-wrap items-center gap-x-6 gap-y-1 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03] ${onRowClick ? "cursor-pointer" : ""}`}
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
