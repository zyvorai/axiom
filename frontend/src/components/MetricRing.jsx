export default function MetricRing({ value = 0, size = 108, stroke = 10, label, sublabel, color = "var(--i17-cosmic)" }) {
    const clamped = Math.max(0, Math.min(100, value));
    const r = (size - stroke) / 2;
    const c = 2 * Math.PI * r;
    const dash = (clamped / 100) * c;

    return (
        <div className="flex items-center gap-4">
            <svg width={size} height={size} className="-rotate-90">
                <defs>
                    <linearGradient id={`g-${label}`} x1="0" y1="0" x2="1" y2="1">
                        <stop offset="0%" stopColor={color} stopOpacity="1" />
                        <stop offset="100%" stopColor={color} stopOpacity="0.4" />
                    </linearGradient>
                </defs>
                <circle
                    cx={size / 2}
                    cy={size / 2}
                    r={r}
                    stroke="currentColor"
                    className="text-black/10 dark:text-white/10"
                    strokeWidth={stroke}
                    fill="none"
                />
                <circle
                    cx={size / 2}
                    cy={size / 2}
                    r={r}
                    stroke={`url(#g-${label})`}
                    strokeLinecap="round"
                    strokeWidth={stroke}
                    fill="none"
                    strokeDasharray={`${dash} ${c - dash}`}
                    style={{ transition: "stroke-dasharray 600ms cubic-bezier(.2,.9,.2,1)" }}
                />
            </svg>
            <div>
                <div className="text-[11px] uppercase tracking-[0.14em] text-muted-foreground">{label}</div>
                <div className="text-3xl font-semibold tracking-[-0.02em] tabular-nums">{clamped}%</div>
                {sublabel ? (
                    <div className="mt-0.5 text-xs text-muted-foreground">{sublabel}</div>
                ) : null}
            </div>
        </div>
    );
}
