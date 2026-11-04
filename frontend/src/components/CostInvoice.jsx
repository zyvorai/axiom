import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { formatCurrency, formatDate } from "@/lib/i18n";
import { api } from "@/lib/api";
import { BILLING as T } from "@/constants/testIds";

const PROVIDER_LABEL = {
    aws: "pages:billing.providers.aws",
    azure: "pages:billing.providers.azure",
    gcp: "pages:billing.providers.gcp",
};

const monthLabel = (m) => formatDate(new Date(`${m}-01T00:00:00Z`), { year: "numeric", month: "long", timeZone: "UTC" });
// Invoices are read to the cent.
const money = (v, currency) => formatCurrency(v, currency, { minimumFractionDigits: 2 });

/**
 * Experimental: the cloud invoice for the current and previous month next to
 * the cluster's allocated cost. Renders nothing until reconciliation is
 * configured (GET /cost/invoice answers configured:false).
 */
export function CostInvoice() {
    const { t } = useTranslation();
    const [data, setData] = useState(null);

    useEffect(() => {
        let alive = true;
        api.get("/cost/invoice")
            .then((r) => alive && setData(r.data))
            .catch(() => alive && setData(null));
        return () => {
            alive = false;
        };
    }, []);

    if (!data || !data.configured) return null;
    const currency = data.currency || "USD";
    const allocatedCurrency = data.allocatedCurrency || "USD";
    const providerKey = PROVIDER_LABEL[data.provider];

    let note = "";
    if (data.demo) note = t("pages:billing.invoice.demo");
    else if (currency !== allocatedCurrency) note = t("pages:billing.invoice.currencyDiffers", { currency });
    else if (data.note) note = data.note;

    return (
        <div className="card p-0" data-testid={T.card}>
            <div className="panel-h flex flex-wrap items-center justify-between gap-2">
                <span className="flex items-center gap-2">
                    {t("pages:billing.invoice.title")}
                    <span className="pill" style={{ "--c": "var(--orange)" }} data-testid={T.badge}>
                        <i aria-hidden="true" />
                        {t("pages:billing.experimental")}
                    </span>
                </span>
                {providerKey && (
                    <span className="text-[12.5px] text-ink-3" data-testid={T.provider}>
                        {t(providerKey)}
                    </span>
                )}
            </div>
            {data.error ? (
                <div className="notice danger m-4" role="alert" data-testid={T.error}>
                    {t("pages:billing.invoice.failed", { error: data.error })}
                </div>
            ) : (
                <table className="tbl">
                    <thead>
                        <tr>
                            <th>{t("pages:billing.invoice.columns.month")}</th>
                            <th>{t("pages:billing.invoice.columns.invoice")}</th>
                            <th>{t("pages:billing.invoice.columns.allocated")}</th>
                            <th>{t("pages:billing.invoice.columns.unallocated")}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {(data.months || []).map((m) => (
                            <tr key={m.month} data-testid={T.row(m.month)}>
                                <td className="name">
                                    {m.partial ? t("pages:billing.invoice.toDate", { month: monthLabel(m.month) }) : monthLabel(m.month)}
                                </td>
                                <td className="mono num" data-testid={T.invoice(m.month)}>{money(m.invoiceTotal, currency)}</td>
                                <td className="mono num" data-testid={T.allocated(m.month)}>{money(m.allocated, allocatedCurrency)}</td>
                                <td className="mono num" data-testid={T.unallocated(m.month)}>
                                    {m.unallocated == null ? "—" : money(m.unallocated, currency)}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            )}
            <div className="px-4 py-3 text-[12.5px] text-ink-3" data-testid={T.note}>
                {t("pages:billing.invoice.explainer")}
                {note ? ` ${note}` : ""}
            </div>
        </div>
    );
}
