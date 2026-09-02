/** Soft canvas wash — Apple light / GitHub dark, no neon orbs. */
export default function AmbientBackdrop() {
    return (
        <div className="pointer-events-none fixed inset-0 -z-10 overflow-hidden bg-background">
            <div
                className="absolute inset-0 opacity-100 dark:opacity-80"
                style={{
                    background:
                        "radial-gradient(ellipse 80% 50% at 50% -10%, hsl(var(--primary) / 0.07), transparent 55%)",
                }}
            />
        </div>
    );
}
