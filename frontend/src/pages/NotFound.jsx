import { Link, useLocation } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { Trans, useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";

// Unknown URLs used to silently redirect to /overview, which hid typos and
// stale links. Show an explicit 404 inside the app shell instead.
export default function NotFound() {
    const { pathname } = useLocation();
    const { t } = useTranslation();
    return (
        <div className="page" data-testid="not-found">
            <PageHeader
                eyebrow="404"
                title={t("pages:notFound.title")}
                description={
                    <Trans i18nKey="pages:notFound.description" values={{ path: pathname }} components={{ path: <span className="mono" /> }} />
                }
            />
            <div className="tools mt-0">
                <Link to="/overview" className="btn btn-fill btn-lg">
                    {t("pages:notFound.goOverview")} <ArrowRight size={16} />
                </Link>
                <button
                    type="button"
                    className="btn btn-line btn-lg"
                    onClick={() => window.dispatchEvent(new Event("axiom:palette"))}
                >
                    {t("common:actions.search")} <kbd className="text-[12px] text-ink-3">⌘K</kbd>
                </button>
            </div>
        </div>
    );
}
