// Apple Store chapter login — full-bleed white hero (brand → title → lede →
// pill CTAs), second chapter for credentials. Ported from
// ../h2kvm-/web/dashboard/src/components/PremiumLoginShell.tsx.
import { AlertCircle } from "lucide-react";
import "../axiom-login.css";

export function PremiumLoginShell({
    logo,
    productName,
    productWordmark,
    productSubtitle,
    heroTitle = "Sign in.",
    heroSubheadline,
    heroCta,
    accent = "sky",
    chapterNote,
    pills,
    panelTitle,
    panelSubtitle,
    panelHint,
    footer,
    formClassName = "",
    showSignInChapter = true,
    children,
    ...rest
}) {
    const tagline = heroSubheadline ?? productSubtitle ?? "";
    const formHeading = panelSubtitle ?? (panelTitle && panelTitle !== "Sign in" ? panelTitle : "Sign in");
    const wordmark = (productWordmark ?? productName ?? "").trim();

    return (
        <div className="login-page login-store-page" {...rest}>
            <main className="login-store-scroll" aria-label="Sign in">
                <section className="login-chapter login-chapter-hero" data-tone={accent} aria-label={productName}>
                    <div className="login-chapter-inner">
                        {logo ? <div className="login-logo inline-flex mb-5">{logo}</div> : null}
                        <p className="login-wordmark" aria-label={productName}>
                            {wordmark}
                        </p>
                        <h1 className="login-hero-title">{heroTitle}</h1>
                        {tagline ? <p className="login-tagline">{tagline}</p> : null}
                        {pills?.length ? (
                            <div className="login-pill-row">
                                {pills.map((pill) => (
                                    <span key={pill.label} data-tone={pill.tone ?? accent} className="login-pill">
                                        <span className="login-pill-dot" aria-hidden="true" />
                                        {pill.icon}
                                        {pill.label}
                                    </span>
                                ))}
                            </div>
                        ) : null}
                        {heroCta ? <div className="login-cta">{heroCta}</div> : null}
                        {chapterNote ? <p className="login-chapter-note">{chapterNote}</p> : null}
                    </div>
                </section>

                {showSignInChapter && children ? (
                    <section id="login-sign-in" className="login-chapter login-chapter-sign-in" aria-label="Credentials">
                        <div className="login-chapter-inner login-sign-in-inner">
                            <p className="login-form-heading">{formHeading}</p>
                            <div className={`login-card ${formClassName}`.trim()}>{children}</div>
                            {panelHint ? <p className="login-hint">{panelHint}</p> : null}
                        </div>
                    </section>
                ) : null}
            </main>

            {footer}
        </div>
    );
}

export function LoginError({ message }) {
    return (
        <div
            className="flex items-start gap-2.5 bg-red-50 border border-red-200 rounded-xl p-3 mb-6 login-shake"
            role="alert"
            aria-live="assertive"
        >
            <AlertCircle className="h-4 w-4 text-red-600 shrink-0 mt-0.5" aria-hidden="true" />
            <div>
                <p className="text-sm font-medium text-red-700">Unable to sign in</p>
                <p className="text-sm text-red-600/90 mt-0.5">{message}</p>
            </div>
        </div>
    );
}

export function LoginField({ label, id, children }) {
    return (
        <div className="mb-4">
            <label htmlFor={id} className="block text-xs font-medium text-zinc-500 mb-1.5">
                {label}
            </label>
            <div className="relative group">{children}</div>
        </div>
    );
}

export function LoginSubmit({ loading, disabled, children, className = "" }) {
    return (
        <button type="submit" disabled={disabled || loading} className={`login-btn-primary ${className}`.trim()}>
            {children}
        </button>
    );
}

export function LoginRemember({ checked, onChange, label = "Remember me on this device", hint }) {
    return (
        <div className="mt-5">
            <label className="flex items-center gap-2.5 cursor-pointer select-none">
                <input
                    type="checkbox"
                    checked={checked}
                    onChange={(e) => onChange(e.target.checked)}
                    className="w-4 h-4 rounded border-zinc-300 accent-[#0071e3]"
                />
                <span className="text-sm text-zinc-500">{label}</span>
            </label>
            {hint ? <p className="text-xs text-zinc-400 mt-1.5 ml-[1.625rem]">{hint}</p> : null}
        </div>
    );
}
