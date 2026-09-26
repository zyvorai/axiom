/** Soft canvas wash — the Netra hero glow (--hero-glow-a/b), no neon orbs. */
export default function AmbientBackdrop() {
    return (
        <div className="pointer-events-none fixed inset-0 -z-10 overflow-hidden bg-background">
            <div
                className="absolute inset-0"
                style={{
                    background:
                        "radial-gradient(ellipse 80% 50% at 30% -10%, var(--hero-glow-a), transparent 60%), radial-gradient(ellipse 60% 40% at 80% -5%, var(--hero-glow-b), transparent 55%)",
                }}
            />
        </div>
    );
}
