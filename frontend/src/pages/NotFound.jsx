import { Link, useLocation } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import PageHeader from "@/components/PageHeader";

// Unknown URLs used to silently redirect to /overview, which hid typos and
// stale links. Show an explicit 404 inside the app shell instead.
export default function NotFound() {
    const { pathname } = useLocation();
    return (
        <div className="page" data-testid="not-found">
            <PageHeader
                eyebrow="404"
                title="Page not found"
                description={
                    <>
                        Nothing lives at <span className="mono">{pathname}</span>. It may have moved, or the link is out of date.
                    </>
                }
            />
            <div className="tools mt-0">
                <Link to="/overview" className="btn btn-fill btn-lg">
                    Go to Overview <ArrowRight size={16} />
                </Link>
                <button
                    type="button"
                    className="btn btn-line btn-lg"
                    onClick={() => window.dispatchEvent(new Event("axiom:palette"))}
                >
                    Search <kbd className="text-[12px] text-ink-3">⌘K</kbd>
                </button>
            </div>
        </div>
    );
}
