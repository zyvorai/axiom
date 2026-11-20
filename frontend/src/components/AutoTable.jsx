import { formatDistanceToNowStrict } from "date-fns";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Badge } from "@/components/ui/badge";
import { useTranslation } from "react-i18next";
import i18n, { dateFnsLocale } from "@/lib/i18n";

const HIDDEN_KEYS = new Set(["id", "cluster_id", "pool_id", "backend_native_id", "connection_ref", "secret_ref"]);
const STATUS_KEYS = ["state", "status", "health"];

function pretty(key) {
    return key.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function formatValue(v) {
    if (v === null || v === undefined) return "—";
    if (typeof v === "boolean") return v ? i18n.t("pages:autoTable.yes") : i18n.t("pages:autoTable.no");
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
    if (!Number.isNaN(parsed)) return formatDistanceToNowStrict(new Date(parsed), { addSuffix: true, locale: dateFnsLocale() });
    return String(raw);
}

function renderColumnValue(row, col) {
    const raw = getPath(row, col.path);
    if (col.type === "age") return formatAge(raw);
    if (col.type === "status") return <StatusBadge status={String(raw ?? "unknown")} />;
    if (col.type === "badge") {
        return (
            <Badge variant="outline" className="text-[11px]">
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
            <h2 className="flex items-center gap-2 text-[15px]">
                <Icon className="h-4 w-4 text-tone-link" /> {title}
            </h2>
            <span className="r text-[12.5px] text-ink-3">{count}</span>
        </div>
    );
}

export default function AutoTable({ title, icon: Icon, items, emptyHint, onRowClick, columns, rowActions }) {
    const { t } = useTranslation();
    const rows = Array.isArray(items) ? items : null;

    if (columns && columns.length > 0) {
        return (
            <GlassCard className="overflow-hidden">
                <TableChrome title={title} Icon={Icon} count={rows ? rows.length : 0} />
                {(!rows || rows.length === 0) ? (
                    <div className="empty">
                    {rows !== null && (
                        <span className="empty-icon" aria-hidden="true">
                            <Icon size={20} />
                        </span>
                    )}
                    {emptyHint}
                </div>
                ) : (
                    <div className="overflow-x-auto" tabIndex={0} role="region" aria-label={title || t("pages:autoTable.dataTable")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    {columns.map((col) => (
                                        <th key={col.path}>{col.label}</th>
                                    ))}
                                    {rowActions && <th><span className="sr-only">{t("common:fields.actions")}</span></th>}
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
            {(!rows || rows.length === 0) && <div className="empty">
                    {rows !== null && (
                        <span className="empty-icon" aria-hidden="true">
                            <Icon size={20} />
                        </span>
                    )}
                    {emptyHint}
                </div>}
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
                                className={onRowClick ? "data-row clickable" : "data-row"}
                            >
                                <div className="data-head">
                                    <div className="data-name" title={String(name)}>{String(name)}</div>
                                    {statusKey && <StatusBadge status={String(row[statusKey])} />}
                                </div>
                                {fields.length > 0 && (
                                    <dl className="data-fields">
                                        {fields.slice(0, 6).map(([k, v]) => {
                                            const val = formatValue(v);
                                            return (
                                                <div key={k}>
                                                    <dt>{pretty(k)}</dt>
                                                    <dd title={val}>{val}</dd>
                                                </div>
                                            );
                                        })}
                                    </dl>
                                )}
                            </div>
                        );
                    })}
            </div>
        </GlassCard>
    );
}
