import { useSyncExternalStore } from "react";
import { AlertTriangle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { getApiFailures, subscribeApiFailures } from "@/lib/apiHealth";

/** One shell-level notice when the backend returned 5xx / didn't respond, with a Retry. */
export default function LoadErrorBanner({ onRetry }) {
    const failures = useSyncExternalStore(subscribeApiFailures, getApiFailures);
    const { t } = useTranslation();
    if (!failures.length) return null;
    const offline = failures.every((f) => f.status === 0);
    return (
        <div className="load-banner">
            <div className="notice danger" role="alert" data-testid="load-error-banner">
                <AlertTriangle size={16} aria-hidden="true" />
                <span>
                    <b>{offline ? t("errors:loadBanner.offlineTitle") : t("errors:loadBanner.failedTitle")}</b>{" "}
                    {offline
                        ? t("errors:loadBanner.offlineHint")
                        : t("errors:loadBanner.failedHint", { count: failures.length })}
                </span>
                <button type="button" className="btn btn-line btn-sm ml-auto" onClick={onRetry}>
                    {t("common:actions.retry")}
                </button>
            </div>
        </div>
    );
}
