export default function AmbientBackdrop() {
    return (
        <div className="pointer-events-none fixed inset-0 -z-10 overflow-hidden">
            <div className="absolute inset-0 grid-bg opacity-60" />
            <div
                className="orb"
                style={{
                    width: 560,
                    height: 560,
                    left: "-8%",
                    top: "-10%",
                    background:
                        "radial-gradient(circle at 30% 30%, rgba(240,88,58,0.55), transparent 60%)",
                }}
            />
            <div
                className="orb"
                style={{
                    width: 480,
                    height: 480,
                    right: "-6%",
                    top: "18%",
                    background:
                        "radial-gradient(circle at 60% 40%, rgba(34,211,238,0.40), transparent 60%)",
                    animationDelay: "-6s",
                }}
            />
            <div
                className="orb"
                style={{
                    width: 620,
                    height: 620,
                    left: "35%",
                    bottom: "-20%",
                    background:
                        "radial-gradient(circle at 40% 60%, rgba(255,143,63,0.28), transparent 60%)",
                    animationDelay: "-11s",
                }}
            />
        </div>
    );
}
