import { useId, useState } from "react";
import { Bookmark, Trash2 } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { VIEWS } from "@/constants/testIds";
import { deleteView, loadViews, saveView } from "@/lib/savedViews";
import { useTranslation } from "react-i18next";

function summary(filters, t) {
    const parts = Object.entries(filters).map(([k, v]) => `${k}=${v}`);
    return parts.length ? parts.join(", ") : t("pages:savedViews.noFilters");
}

/**
 * Per-page saved filter sets. `filters` is the current { urlParam: value } snapshot (empty
 * values are dropped when saving); `onApply(filters)` writes a saved snapshot back to the page's
 * URL state.
 */
export default function SavedViews({ pageKey, filters, onApply }) {
    const { t } = useTranslation();
    const [views, setViews] = useState(() => loadViews(pageKey));
    const [open, setOpen] = useState(false);
    const [name, setName] = useState("");
    const nameId = useId();

    const save = (e) => {
        e.preventDefault();
        if (!name.trim()) return;
        setViews(saveView(pageKey, name, filters));
        setName("");
    };

    return (
        <Popover
            open={open}
            onOpenChange={(o) => {
                setOpen(o);
                if (o) setViews(loadViews(pageKey));
            }}
        >
            <PopoverTrigger asChild>
                <button type="button" className="btn btn-line h-[40px]" data-testid={VIEWS.trigger} aria-label={t("pages:savedViews.title")} title={t("pages:savedViews.title")}>
                    <Bookmark size={14} aria-hidden="true" />
                    <span className="hidden sm:inline">{views.length ? t("pages:savedViews.viewsWithCount", { n: views.length }) : t("pages:savedViews.views")}</span>
                </button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-80 p-3" data-testid={VIEWS.panel}>
                <div className="mb-2 text-[12.5px] font-medium">{t("pages:savedViews.title")}</div>
                {views.length === 0 ? (
                    <p className="mb-3 text-[12.5px] text-ink-3">{t("pages:savedViews.empty")}</p>
                ) : (
                    <ul className="mb-3 max-h-56 space-y-1 overflow-y-auto" tabIndex={0} aria-label={t("pages:savedViews.listAria")}>
                        {views.map((v) => (
                            <li key={v.name} className="flex items-center gap-1">
                                <button
                                    type="button"
                                    data-testid={VIEWS.item(v.name)}
                                    className="min-w-0 flex-1 rounded-lg px-2 py-1.5 text-left hover:bg-muted"
                                    onClick={() => {
                                        onApply(v.filters);
                                        setOpen(false);
                                    }}
                                >
                                    <div className="truncate text-[13px]">{v.name}</div>
                                    <div className="mono truncate text-[11px] text-ink-3">{summary(v.filters, t)}</div>
                                </button>
                                <button
                                    type="button"
                                    data-testid={VIEWS.remove(v.name)}
                                    className="ic !h-8 !w-8"
                                    aria-label={t("pages:savedViews.deleteAria", { name: v.name })}
                                    title={t("pages:savedViews.deleteTitle")}
                                    onClick={() => setViews(deleteView(pageKey, v.name))}
                                >
                                    <Trash2 size={13} aria-hidden="true" />
                                </button>
                            </li>
                        ))}
                    </ul>
                )}
                <form onSubmit={save} className="flex items-center gap-2">
                    <label htmlFor={nameId} className="sr-only">
                        {t("pages:savedViews.nameLabel")}
                    </label>
                    <input
                        id={nameId}
                        data-testid={VIEWS.nameInput}
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        placeholder={t("pages:savedViews.namePlaceholder")}
                        maxLength={60}
                        className="input min-w-0 flex-1 !h-9 bg-transparent px-3 text-[13px]"
                    />
                    <button type="submit" data-testid={VIEWS.save} className="btn btn-line btn-sm" disabled={!name.trim()}>
                        {t("common:actions.save")}
                    </button>
                </form>
            </PopoverContent>
        </Popover>
    );
}
