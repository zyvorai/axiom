import { useId, useState } from "react";
import { PlugZap, Receipt, Save } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { BILLING_SETTINGS as T } from "@/constants/testIds";

const SELECT = "bg-muted/30 mt-1.5 h-9 w-full rounded-md border border-border px-3 text-sm";

/**
 * Settings card for the experimental actual-bill reconciliation. Credentials are
 * write-only: the server only returns `...Set` flags, and a blank field keeps
 * the stored value.
 */
export default function BillingSettingsCard({ data, onSaved }) {
    const { t } = useTranslation();
    const uid = useId();
    const [form, setForm] = useState(() => ({
        provider: data.billingProvider || "",
        tagKey: data.billingTagKey || "",
        tagValue: data.billingTagValue || "",
        awsRegion: data.billingAwsRegion || "us-east-1",
        awsAccessKeyId: "",
        awsSecretAccessKey: "",
        awsSessionToken: "",
        azureTenantId: data.billingAzureTenantId || "",
        azureClientId: data.billingAzureClientId || "",
        azureSubscriptionId: data.billingAzureSubscriptionId || "",
        azureClientSecret: "",
        gcpProjectId: data.billingGcpProjectId || "",
        gcpTable: data.billingGcpTable || "",
        gcpServiceAccountKey: "",
    }));
    const [busy, setBusy] = useState("");
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));
    const secret = (key, value) => (value.trim() ? { [key]: value } : {});

    const save = async () => {
        setBusy("save");
        try {
            const { data: v } = await api.put("/integrations", {
                billingProvider: form.provider,
                billingTagKey: form.tagKey,
                billingTagValue: form.tagValue,
                billingAwsRegion: form.awsRegion,
                billingAzureTenantId: form.azureTenantId,
                billingAzureClientId: form.azureClientId,
                billingAzureSubscriptionId: form.azureSubscriptionId,
                billingGcpProjectId: form.gcpProjectId,
                billingGcpTable: form.gcpTable,
                ...secret("billingAwsAccessKeyId", form.awsAccessKeyId),
                ...secret("billingAwsSecretAccessKey", form.awsSecretAccessKey),
                ...secret("billingAwsSessionToken", form.awsSessionToken),
                ...secret("billingAzureClientSecret", form.azureClientSecret),
                ...secret("billingGcpServiceAccountKey", form.gcpServiceAccountKey),
            });
            onSaved?.(v);
            set({ awsAccessKeyId: "", awsSecretAccessKey: "", awsSessionToken: "", azureClientSecret: "", gcpServiceAccountKey: "" });
            toast.success(t("toasts:billing.saved"));
        } catch (e) {
            toast.error(t("toasts:billing.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const test = async () => {
        setBusy("test");
        try {
            const { data: r } = await api.post("/integrations/billing/test", null, { timeout: 60000 });
            r.ok
                ? toast.success(t("toasts:billing.reachable"), { description: r.hint })
                : toast.error(t("toasts:billing.testFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:billing.testFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const field = (key, labelKey, testId, props = {}) => (
        <div>
            <Label htmlFor={`${uid}-${key}`} className="text-xs text-muted-foreground">{t(labelKey)}</Label>
            <Input id={`${uid}-${key}`} data-testid={testId} className="bg-muted/30 mt-1.5" value={form[key]} onChange={(e) => set({ [key]: e.target.value })} {...props} />
        </div>
    );
    const secretField = (key, isSet, labelKey, testId) =>
        field(key, isSet ? "pages:billing.settings.secretSet" : labelKey, testId, { type: "password", autoComplete: "off", "aria-label": t(labelKey) });

    return (
        <div className="card" data-testid={T.card}>
            <div className="form-section mb-0">
                <div className="flex items-start justify-between gap-3">
                    <h2 className="flex items-center gap-2">
                        <Receipt size={16} />
                        {t("pages:billing.settings.title")}
                    </h2>
                    <span className="pill" style={{ "--c": "var(--orange)" }}>
                        <i aria-hidden="true" />
                        {t("pages:billing.experimental")}
                    </span>
                </div>
                <p className="lead">{t("pages:billing.settings.lead")}</p>

                <div className="form-row">
                    <div>
                        <Label htmlFor={`${uid}-provider`} className="text-xs text-muted-foreground">{t("pages:billing.settings.provider")}</Label>
                        <select id={`${uid}-provider`} data-testid={T.provider} value={form.provider} onChange={(e) => set({ provider: e.target.value })} className={SELECT}>
                            <option value="">{t("pages:billing.settings.off")}</option>
                            <option value="aws">{t("pages:billing.providers.aws")}</option>
                            <option value="azure">{t("pages:billing.providers.azure")}</option>
                            <option value="gcp">{t("pages:billing.providers.gcp")}</option>
                        </select>
                    </div>
                    {field("tagKey", "pages:billing.settings.tagKey", T.tagKey, { placeholder: t("pages:billing.settings.tagKeyPlaceholder") })}
                    {field("tagValue", "pages:billing.settings.tagValue", T.tagValue)}
                </div>

                {form.provider === "aws" && (
                    <div className="form-row">
                        <div>
                            <Label htmlFor={`${uid}-awsRegion`} className="text-xs text-muted-foreground">{t("pages:billing.settings.awsRegion")}</Label>
                            <select id={`${uid}-awsRegion`} data-testid={T.awsRegion} value={form.awsRegion} onChange={(e) => set({ awsRegion: e.target.value })} className={SELECT}>
                                <option value="us-east-1">us-east-1</option>
                                <option value="cn-northwest-1">cn-northwest-1</option>
                            </select>
                        </div>
                        {secretField("awsAccessKeyId", data.billingAwsAccessKeyIdSet, "pages:billing.settings.awsAccessKeyId", T.awsAccessKeyId)}
                        {secretField("awsSecretAccessKey", data.billingAwsSecretAccessKeySet, "pages:billing.settings.awsSecretAccessKey", T.awsSecretAccessKey)}
                        {secretField("awsSessionToken", data.billingAwsSessionTokenSet, "pages:billing.settings.awsSessionToken", T.awsSessionToken)}
                    </div>
                )}

                {form.provider === "azure" && (
                    <div className="form-row">
                        {field("azureTenantId", "pages:billing.settings.azureTenantId", T.azureTenantId)}
                        {field("azureClientId", "pages:billing.settings.azureClientId", T.azureClientId)}
                        {field("azureSubscriptionId", "pages:billing.settings.azureSubscriptionId", T.azureSubscriptionId)}
                        {secretField("azureClientSecret", data.billingAzureClientSecretSet, "pages:billing.settings.azureClientSecret", T.azureClientSecret)}
                    </div>
                )}

                {form.provider === "gcp" && (
                    <>
                        <div className="form-row">
                            {field("gcpTable", "pages:billing.settings.gcpTable", T.gcpTable, { placeholder: "my-project.billing_export.gcp_billing_export_v1_XXXXXX" })}
                            {field("gcpProjectId", "pages:billing.settings.gcpProjectId", T.gcpProjectId)}
                        </div>
                        <div className="mb-4">
                            <Label htmlFor={`${uid}-gcpKey`} className="text-xs text-muted-foreground">
                                {data.billingGcpServiceAccountKeySet ? t("pages:billing.settings.secretSet") : t("pages:billing.settings.gcpServiceAccountKey")}
                            </Label>
                            <Textarea
                                id={`${uid}-gcpKey`}
                                data-testid={T.gcpServiceAccountKey}
                                className="bg-muted/30 mt-1.5 mono text-[12px]"
                                rows={4}
                                autoComplete="off"
                                spellCheck={false}
                                value={form.gcpServiceAccountKey}
                                onChange={(e) => set({ gcpServiceAccountKey: e.target.value })}
                            />
                        </div>
                    </>
                )}

                <p className="text-[12.5px] text-ink-3 mb-4">{t("pages:billing.settings.permissions")}</p>

                <div className="flex flex-wrap gap-2.5">
                    <button type="button" data-testid={T.save} className="btn btn-fill btn-sm" onClick={save} disabled={busy === "save"}>
                        <Save size={14} /> {busy === "save" ? t("common:actions.saving") : t("common:actions.save")}
                    </button>
                    <button type="button" data-testid={T.test} className="btn btn-line btn-sm" onClick={test} disabled={busy === "test"}>
                        <PlugZap size={14} /> {busy === "test" ? t("pages:settings.testing") : t("pages:settings.testConnection")}
                    </button>
                </div>
            </div>
        </div>
    );
}
