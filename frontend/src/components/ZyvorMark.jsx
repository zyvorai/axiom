// zyvor.dev's actual brand mark: an orange gradient tile with a white Z
// stroke, matching the logo used on zyvor.dev itself. Deliberately a fixed
// brand orange rather than the app's own theme accent (--apple-blue) — this
// identifies the parent Zyvor brand, not the Axiom product, so it doesn't
// change with the light/dark theme toggle.
export default function ZyvorMark({ size = 36, className = "", idPrefix = "zyvor-mark" }) {
    const grad = `${idPrefix}-bg`;
    return (
        <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 64 64"
            width={size}
            height={size}
            className={className}
            role="img"
            aria-label="Zyvor"
        >
            <defs>
                <linearGradient id={grad} x1="8" y1="8" x2="56" y2="56" gradientUnits="userSpaceOnUse">
                    <stop offset="0" stopColor="hsl(28 96% 56%)" />
                    <stop offset="1" stopColor="hsl(20 92% 50%)" />
                </linearGradient>
            </defs>
            <rect width="64" height="64" rx="15" fill={`url(#${grad})`} />
            <path
                d="M18.5,20.5 45.5,20.5 18.5,43.5 45.5,43.5"
                fill="none"
                stroke="var(--on-accent)"
                strokeWidth="8.3"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}
