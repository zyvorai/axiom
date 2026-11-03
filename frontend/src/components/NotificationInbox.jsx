import { Link } from "react-router-dom";
import { AlertTriangle, Bell, Check, X } from "lucide-react";
import { formatDistanceToNowStrict } from "date-fns";
import { useTranslation } from "react-i18next";
import { dateFnsLocale } from "@/lib/i18n";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { INBOX } from "@/constants/testIds";
import { useInbox } from "@/lib/inbox";
import { resourceHref } from "@/lib/resourceHref";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return "";
    }
}

/** Top-bar bell: a panel of recent cluster warnings with local read/dismiss state. */
export default function NotificationInbox() {
    const { t } = useTranslation();
    const { items, unread, markRead, markAllRead, dismiss, dismissAll } = useInbox();

    return (
        <Popover>
            <PopoverTrigger asChild>
                <button
                    type="button"
                    className="ic"
                    data-testid={INBOX.bell}
                    aria-label={unread > 0 ? t("pages:notifications.bellUnread", { n: unread }) : t("pages:notifications.title")}
                    title={t("pages:notifications.title")}
                >
                    <Bell size={16} aria-hidden="true" />
                    {unread > 0 && <span className="dot" aria-hidden="true" />}
                </button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-[380px] max-w-[calc(100vw-24px)] p-0" data-testid={INBOX.panel} aria-label={t("pages:notifications.title")}>
                <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-3">
                    <div className="text-[13.5px] font-medium">
                        {t("pages:notifications.title")}
                        {unread > 0 && <span className="ml-1.5 text-ink-3">{t("pages:notifications.unread", { n: unread })}</span>}
                    </div>
                    <div className="flex items-center gap-1">
                        <button type="button" data-testid={INBOX.markAllRead} className="btn btn-line btn-sm" onClick={markAllRead} disabled={unread === 0}>
                            <Check size={13} aria-hidden="true" /> {t("pages:notifications.markAllRead")}
                        </button>
                        <button type="button" className="btn btn-line btn-sm" onClick={dismissAll} disabled={items.length === 0}>
                            {t("common:actions.clear")}
                        </button>
                    </div>
                </div>
                <div className="max-h-[360px] overflow-y-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:notifications.recent")}>
                    {items.length === 0 ? (
                        <div data-testid={INBOX.empty} className="px-4 py-8 text-center text-[13px] text-ink-3">
                            {t("pages:notifications.empty")}
                        </div>
                    ) : (
                        <ul>
                            {items.map((w, i) => {
                                const href = resourceHref(w.namespace, w.object);
                                return (
                                    <li
                                        key={w.id}
                                        data-testid={INBOX.item(i)}
                                        data-read={w.read ? "true" : "false"}
                                        className="flex items-start gap-2.5 border-b border-border px-4 py-3 last:border-none"
                                    >
                                        <AlertTriangle size={14} className="mt-0.5 shrink-0 text-tone-orange" aria-hidden="true" />
                                        <div className="min-w-0 flex-1 text-[12.5px]">
                                            <div className={w.read ? "text-ink-2" : "font-medium"}>
                                                <span className="mono text-tone-link">{w.reason}</span> — {w.message}
                                            </div>
                                            <div className="mt-1 flex flex-wrap items-center gap-x-2 text-[11.5px] text-ink-3">
                                                {href ? (
                                                    <Link to={href} className="mono text-tone-link hover:underline" onClick={() => markRead(w.id)}>
                                                        {w.namespace}/{w.object}
                                                    </Link>
                                                ) : (
                                                    <span className="mono">
                                                        {w.namespace}/{w.object}
                                                    </span>
                                                )}
                                                <span>{relTime(w.time)}</span>
                                                {!w.read && (
                                                    <button type="button" className="underline" onClick={() => markRead(w.id)}>
                                                        {t("pages:notifications.markRead")}
                                                    </button>
                                                )}
                                            </div>
                                        </div>
                                        <button
                                            type="button"
                                            data-testid={INBOX.dismiss(i)}
                                            className="ic !h-7 !w-7 shrink-0"
                                            aria-label={t("pages:notifications.dismissAria", { reason: w.reason, object: w.object })}
                                            title={t("pages:notifications.dismiss")}
                                            onClick={() => dismiss(w.id)}
                                        >
                                            <X size={13} aria-hidden="true" />
                                        </button>
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </div>
                <div className="border-t border-border px-4 py-2.5 text-[12.5px]">
                    <Link to="/alerts" className="text-tone-link hover:underline">
                        {t("pages:notifications.viewAll")}
                    </Link>
                </div>
            </PopoverContent>
        </Popover>
    );
}
