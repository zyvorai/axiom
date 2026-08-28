import {
    Area,
    AreaChart,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from "recharts";

export default function Sparkline({ data, color = "#0A84FF", formatter, domain }) {
    const rows = (data || []).map((v, i) => ({ i, v }));
    const fmt = formatter || ((v) => `${v}%`);
    return (
        <ResponsiveContainer width="100%" height={80}>
            <AreaChart data={rows} margin={{ top: 5, right: 0, left: 0, bottom: 0 }}>
                <defs>
                    <linearGradient id={`sp-${color}`} x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor={color} stopOpacity={0.55} />
                        <stop offset="100%" stopColor={color} stopOpacity={0} />
                    </linearGradient>
                </defs>
                <Area
                    type="monotone"
                    dataKey="v"
                    stroke={color}
                    strokeWidth={2}
                    fill={`url(#sp-${color})`}
                    isAnimationActive={false}
                />
                <XAxis dataKey="i" hide />
                <YAxis hide domain={domain || [0, 100]} />
                <Tooltip
                    contentStyle={{
                        background: "rgba(20,20,22,0.85)",
                        border: "1px solid rgba(255,255,255,0.1)",
                        borderRadius: 12,
                        color: "#fff",
                    }}
                    formatter={fmt}
                    labelFormatter={() => ""}
                />
            </AreaChart>
        </ResponsiveContainer>
    );
}
