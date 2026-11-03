import { useId, useState } from "react";
import { PlugZap, Save, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { useTranslation, Trans } from "react-i18next";
import { api } from "@/lib/api";
import { AI_PROVIDERS } from "@/lib/ai";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { AI_SETTINGS } from "@/constants/testIds";

/**
 * Settings card for the opt-in AI assistant. Self-contained so Settings.jsx only
 * mounts it: `<AiSettingsCard data={data} onSaved={...} />`. The API key is write-only —
 * the server returns just `aiApiKeySet`.
 */
export default function AiSettingsCard({ data, onSaved }) {
    const { t } = useTranslation();
    const uid = useId();
    const [form, setForm] = useState(() => ({
        enabled: !!data.aiEnabled,
        provider: data.aiProvider || "openai-compatible",
        baseUrl: data.aiBaseUrl || "",
        model: data.aiModel || "",
        apiKey: "",
        redact: data.aiRedact !== false,
    }));
    const [busy, setBusy] = useState("");
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));
    const provider = AI_PROVIDERS.find((p) => p.value === form.provider) || AI_PROVIDERS[0];

    const save = async () => {
        setBusy("save");
        try {
            const { data: v } = await api.put("/integrations", {
                aiEnabled: form.enabled,
                aiProvider: form.provider,
                aiBaseUrl: form.baseUrl,
                aiModel: form.model,
                aiRedact: form.redact,
                ...(form.apiKey ? { aiApiKey: form.apiKey } : {}),
            });
            onSaved?.(v);
            set({ apiKey: "" });
            toast.success(t("toasts:aiSettings.saved"));
        } catch (e) {
            toast.error(t("toasts:aiSettings.saveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const test = async () => {
        setBusy("test");
        try {
            const { data: r } = await api.post("/integrations/ai/test", null, { timeout: 15000 });
            r.ok
                ? toast.success(t("toasts:aiSettings.reachable"), { description: r.hint || (r.latencyMs ? `${r.latencyMs}ms` : undefined) })
                : toast.error(t("toasts:aiSettings.aiTestFailed"), { description: r.error || r.hint });
        } catch (e) {
            toast.error(t("toasts:aiSettings.testFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <div className="card" data-testid={AI_SETTINGS.card}>
            <div className="form-section mb-0">
                <div className="flex items-start justify-between gap-3">
                    <h2 className="flex items-center gap-2">
                        <Sparkles size={16} />
                        {t("pages:aiSettings.title")}
                    </h2>
                    <span className="pill" style={{ "--c": data.aiEnabled && data.aiConfigured ? "var(--green)" : "var(--ink-3)" }}>
                        <i aria-hidden="true" />
                        {data.aiEnabled && data.aiConfigured ? t("pages:aiSettings.pillEnabled") : t("pages:aiSettings.pillOff")}
                    </span>
                </div>
                <p className="lead">
                    <Trans i18nKey="pages:aiSettings.lead" components={{ mono: <span className="mono" /> }} />
                </p>

                <div className="rule-row mb-4">
                    <div>
                        <div className="text-[14px]">{t("pages:aiSettings.enable")}</div>
                        <div className="text-[12.5px] text-ink-3">{t("pages:aiSettings.enableHint")}</div>
                    </div>
                    <Switch
                        aria-label={t("pages:aiSettings.enable")}
                        data-testid={AI_SETTINGS.enabled}
                        checked={form.enabled}
                        onCheckedChange={(v) => set({ enabled: v })}
                    />
                </div>

                <div className="form-row">
                    <div>
                        <Label htmlFor={`${uid}-provider`} className="text-xs text-muted-foreground">{t("pages:aiSettings.provider")}</Label>
                        <select
                            id={`${uid}-provider`}
                            data-testid={AI_SETTINGS.provider}
                            value={form.provider}
                            onChange={(e) => set({ provider: e.target.value })}
                            className="bg-muted/30 mt-1.5 h-9 w-full rounded-md border border-border px-3 text-sm"
                        >
                            {AI_PROVIDERS.map((p) => (
                                <option key={p.value} value={p.value}>{p.label}</option>
                            ))}
                        </select>
                    </div>
                    <div>
                        <Label htmlFor={`${uid}-base`} className="text-xs text-muted-foreground">{t("pages:aiSettings.baseUrl")}</Label>
                        <Input
                            id={`${uid}-base`}
                            data-testid={AI_SETTINGS.baseUrl}
                            className="bg-muted/30 mt-1.5"
                            value={form.baseUrl}
                            onChange={(e) => set({ baseUrl: e.target.value })}
                            placeholder={provider.placeholder}
                        />
                    </div>
                    <div>
                        <Label htmlFor={`${uid}-model`} className="text-xs text-muted-foreground">{t("pages:aiSettings.model")}</Label>
                        <Input
                            id={`${uid}-model`}
                            data-testid={AI_SETTINGS.model}
                            className="bg-muted/30 mt-1.5"
                            value={form.model}
                            onChange={(e) => set({ model: e.target.value })}
                            placeholder={t("pages:aiSettings.modelPlaceholder")}
                        />
                    </div>
                    <div>
                        <Label htmlFor={`${uid}-key`} className="text-xs text-muted-foreground">
                            {data.aiApiKeySet ? t("pages:aiSettings.apiKeySet") : t("pages:aiSettings.apiKey")}
                        </Label>
                        <Input
                            id={`${uid}-key`}
                            data-testid={AI_SETTINGS.apiKey}
                            className="bg-muted/30 mt-1.5"
                            type="password"
                            autoComplete="off"
                            value={form.apiKey}
                            onChange={(e) => set({ apiKey: e.target.value })}
                        />
                    </div>
                </div>

                <div className="rule-row mb-4">
                    <div>
                        <div className="text-[14px]">{t("pages:aiSettings.mask")}</div>
                        <div className="text-[12.5px] text-ink-3">{t("pages:aiSettings.maskHint")}</div>
                    </div>
                    <Switch
                        aria-label={t("pages:aiSettings.maskAria")}
                        data-testid={AI_SETTINGS.redact}
                        checked={form.redact}
                        onCheckedChange={(v) => set({ redact: v })}
                    />
                </div>

                <div className="flex flex-wrap gap-2.5">
                    <button type="button" data-testid={AI_SETTINGS.save} className="btn btn-fill btn-sm" onClick={save} disabled={busy === "save"}>
                        <Save size={14} /> {busy === "save" ? t("common:actions.saving") : t("common:actions.save")}
                    </button>
                    <button type="button" data-testid={AI_SETTINGS.test} className="btn btn-line btn-sm" onClick={test} disabled={busy === "test"}>
                        <PlugZap size={14} /> {busy === "test" ? t("pages:aiSettings.testing") : t("pages:aiSettings.testConnection")}
                    </button>
                </div>
            </div>
        </div>
    );
}
