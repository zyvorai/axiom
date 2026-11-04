import { Component } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle } from "lucide-react";
import i18n from "@/lib/i18n";

// Catches render-time exceptions in the page outlet. Before this, one bad response shape (or a
// lazy chunk that no longer exists after a redeploy) blanked the whole app with no way back.
// `resetKey` changes on navigation/retry and clears the error.
const isChunkError = (e) =>
    e && (e.name === "ChunkLoadError" || /Loading (CSS )?chunk|dynamically imported module|Failed to fetch dynamically/i.test(e.message || ""));

export default class ErrorBoundary extends Component {
    state = { error: null };

    static getDerivedStateFromError(error) {
        return { error };
    }

    componentDidUpdate(prev) {
        if (this.state.error && prev.resetKey !== this.props.resetKey) this.setState({ error: null });
    }

    render() {
        const { error } = this.state;
        if (!error) return this.props.children;
        const chunk = isChunkError(error);
        const t = i18n.t.bind(i18n);
        return (
            <div className="page">
                <div className="card error-panel" role="alert" data-testid="error-boundary">
                    <AlertTriangle size={28} aria-hidden="true" />
                    <h2>{chunk ? t("errors:boundary.updatedTitle") : t("errors:boundary.crashTitle")}</h2>
                    <p>
                        {chunk
                            ? t("errors:boundary.updatedHint")
                            : t("errors:boundary.crashHint")}
                    </p>
                    {!chunk && error.message && <code className="mono">{String(error.message).slice(0, 200)}</code>}
                    <div className="tools">
                        {chunk ? (
                            <button type="button" className="btn btn-fill btn-lg" onClick={() => window.location.reload()}>
                                {t("common:actions.reload")}
                            </button>
                        ) : (
                            <button type="button" className="btn btn-fill btn-lg" onClick={() => this.props.onRetry?.()}>
                                {t("common:actions.tryAgain")}
                            </button>
                        )}
                        <Link to="/overview" className="btn btn-line btn-lg">
                            {t("errors:boundary.goOverview")}
                        </Link>
                    </div>
                </div>
            </div>
        );
    }
}
