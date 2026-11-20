import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";

/** Simple SVG sparkline charts for Prometheus history (CPU / memory). */
export default function MetricHistoryChart({ namespace, name, kind = "pod", window: win = "1h" }) {
    const { t } = useTranslation();
    const [data, setData] = useState(null);

    useEffect(() => {
        let alive = true;
        api
            .get("/metrics/history", { params: { namespace, name, kind, window: win } })
            .then((r) => alive && setData(r.data))
            .catch(() => alive && setData({ available: false }));
        return () => { alive = false; };
    }, [namespace, name, kind, win]);

    const cpu = useMemo(() => (data?.series || []).filter((s) => String(s.name).startsWith("cpu:")), [data]);
    const mem = useMemo(() => (data?.series || []).filter((s) => String(s.name).startsWith("mem:")), [data]);
    const gpu = useMemo(() => (data?.series || []).filter((s) => String(s.name).startsWith("gpu:")), [data]);

    if (!data) return <div className="text-[12.5px] text-ink-3">{t("common:states.loading")}</div>;
    if (!data.available) {
        return <div className="text-[12.5px] text-ink-3">{data.note || t("pages:metrics.unavailable")}</div>;
    }
    if (data.note && (!data.series || data.series.length === 0)) {
        return <div className="text-[12.5px] text-ink-3">{data.note}</div>;
    }

    return (
        <div className={`grid gap-3 ${gpu.length ? "sm:grid-cols-3" : "sm:grid-cols-2"}`}>
            <Spark title={t("pages:metrics.cpu")} series={cpu} />
            <Spark title={t("pages:metrics.memory")} series={mem} scale={(v) => v / (1024 * 1024)} unit="MiB" />
            {gpu.length > 0 && <Spark title={t("pages:metrics.gpu", { defaultValue: "GPU util" })} series={gpu} unit="%" />}
        </div>
    );
}

function Spark({ title, series, scale = (v) => v, unit = "" }) {
    const pts = (series[0]?.points || []).map((p) => ({ t: p.t, v: scale(p.v) }));
    if (pts.length < 2) {
        return (
            <div className="rounded-xl border border-border p-3">
                <div className="mb-2 text-[11px] uppercase tracking-widest text-ink-3">{title}</div>
                <div className="text-[12.5px] text-ink-3">—</div>
            </div>
        );
    }
    const min = Math.min(...pts.map((p) => p.v));
    const max = Math.max(...pts.map((p) => p.v));
    const span = max - min || 1;
    const w = 240;
    const h = 56;
    const d = pts
        .map((p, i) => {
            const x = (i / (pts.length - 1)) * w;
            const y = h - ((p.v - min) / span) * (h - 4) - 2;
            return `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`;
        })
        .join(" ");
    const last = pts[pts.length - 1].v;
    return (
        <div className="rounded-xl border border-border p-3">
            <div className="mb-2 flex items-baseline justify-between">
                <div className="text-[11px] uppercase tracking-widest text-ink-3">{title}</div>
                <div className="mono text-[12px]">{last.toFixed(unit === "MiB" ? 0 : 3)}{unit ? ` ${unit}` : ""}</div>
            </div>
            <svg viewBox={`0 0 ${w} ${h}`} className="h-14 w-full text-tone-link" aria-hidden="true">
                <path d={d} fill="none" stroke="currentColor" strokeWidth="1.5" />
            </svg>
        </div>
    );
}
