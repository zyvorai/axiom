import { useSyncExternalStore } from "react";
import { AlertTriangle } from "lucide-react";
import { getApiFailures, subscribeApiFailures } from "@/lib/apiHealth";

/** One shell-level notice when the backend returned 5xx / didn't respond, with a Retry. */
export default function LoadErrorBanner({ onRetry }) {
    const failures = useSyncExternalStore(subscribeApiFailures, getApiFailures);
    if (!failures.length) return null;
    const offline = failures.every((f) => f.status === 0);
    return (
        <div className="load-banner">
            <div className="notice danger" role="alert" data-testid="load-error-banner">
                <AlertTriangle size={16} aria-hidden="true" />
                <span>
                    <b>{offline ? "Can't reach the server." : "Some data couldn't be loaded."}</b>{" "}
                    {offline
                        ? "Check your connection — this page may be out of date."
                        : `The server returned an error (${failures.length} request${failures.length === 1 ? "" : "s"}). What you see may be incomplete.`}
                </span>
                <button type="button" className="btn btn-line btn-sm ml-auto" onClick={onRetry}>
                    Retry
                </button>
            </div>
        </div>
    );
}
