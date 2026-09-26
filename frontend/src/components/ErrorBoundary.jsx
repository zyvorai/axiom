import { Component } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle } from "lucide-react";

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
        return (
            <div className="page">
                <div className="card error-panel" role="alert" data-testid="error-boundary">
                    <AlertTriangle size={28} aria-hidden="true" />
                    <h2>{chunk ? "Axiom was updated" : "This page hit a problem"}</h2>
                    <p>
                        {chunk
                            ? "A newer version is available, so this page couldn't load. Reload to get it."
                            : "Something went wrong while showing this page. The rest of the app is fine."}
                    </p>
                    {!chunk && error.message && <code className="mono">{String(error.message).slice(0, 200)}</code>}
                    <div className="tools">
                        {chunk ? (
                            <button type="button" className="btn btn-fill btn-lg" onClick={() => window.location.reload()}>
                                Reload
                            </button>
                        ) : (
                            <button type="button" className="btn btn-fill btn-lg" onClick={() => this.props.onRetry?.()}>
                                Try again
                            </button>
                        )}
                        <Link to="/overview" className="btn btn-line btn-lg">
                            Go to Overview
                        </Link>
                    </div>
                </div>
            </div>
        );
    }
}
