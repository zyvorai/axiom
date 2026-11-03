import { useId, useState } from "react";
import { PlugZap, Save, Sparkles } from "lucide-react";
import { toast } from "sonner";
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
            toast.success("AI assistant settings saved");
        } catch (e) {
            toast.error("Save failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const test = async () => {
        setBusy("test");
        try {
            const { data: r } = await api.post("/integrations/ai/test", null, { timeout: 15000 });
            r.ok
                ? toast.success("AI endpoint reachable", { description: r.hint || (r.latencyMs ? `${r.latencyMs}ms` : undefined) })
                : toast.error("AI test failed", { description: r.error || r.hint });
        } catch (e) {
            toast.error("Test failed", { description: e?.response?.data?.error || e.message });
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
                        AI assistant
                    </h2>
                    <span className="pill" style={{ "--c": data.aiEnabled && data.aiConfigured ? "var(--green)" : "var(--ink-3)" }}>
                        <i aria-hidden="true" />
                        {data.aiEnabled && data.aiConfigured ? "enabled" : "off"}
                    </span>
                </div>
                <p className="lead">
                    Optional &quot;Explain with AI&quot; on pods and Warning events, using your own model endpoint. Off by default. Each request sends a
                    redacted snapshot (events, container status, a short log tail; never Secret or ConfigMap values or env values) to the endpoint below
                    and is audited as <span className="mono">ai.explain</span> (target only). The base URL is an outbound destination chosen by an admin.
                </p>

                <div className="rule-row mb-4">
                    <div>
                        <div className="text-[14px]">Enable AI assistant</div>
                        <div className="text-[12.5px] text-ink-3">When off, the buttons are hidden and the API refuses requests.</div>
                    </div>
                    <Switch
                        aria-label="Enable AI assistant"
                        data-testid={AI_SETTINGS.enabled}
                        checked={form.enabled}
                        onCheckedChange={(v) => set({ enabled: v })}
                    />
                </div>

                <div className="form-row">
                    <div>
                        <Label htmlFor={`${uid}-provider`} className="text-xs text-muted-foreground">Provider</Label>
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
                        <Label htmlFor={`${uid}-base`} className="text-xs text-muted-foreground">Base URL</Label>
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
                        <Label htmlFor={`${uid}-model`} className="text-xs text-muted-foreground">Model</Label>
                        <Input
                            id={`${uid}-model`}
                            data-testid={AI_SETTINGS.model}
                            className="bg-muted/30 mt-1.5"
                            value={form.model}
                            onChange={(e) => set({ model: e.target.value })}
                            placeholder="e.g. gpt-4o-mini, claude-sonnet-4-5, llama3.1"
                        />
                    </div>
                    <div>
                        <Label htmlFor={`${uid}-key`} className="text-xs text-muted-foreground">
                            {data.aiApiKeySet ? "API key (set — leave blank to keep)" : "API key"}
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
                        <div className="text-[14px]">Also mask IP addresses and hostnames</div>
                        <div className="text-[12.5px] text-ink-3">Recommended. Secrets, tokens and env values are always removed.</div>
                    </div>
                    <Switch
                        aria-label="Mask IP addresses and hostnames"
                        data-testid={AI_SETTINGS.redact}
                        checked={form.redact}
                        onCheckedChange={(v) => set({ redact: v })}
                    />
                </div>

                <div className="flex flex-wrap gap-2.5">
                    <button type="button" data-testid={AI_SETTINGS.save} className="btn btn-fill btn-sm" onClick={save} disabled={busy === "save"}>
                        <Save size={14} /> {busy === "save" ? "Saving…" : "Save"}
                    </button>
                    <button type="button" data-testid={AI_SETTINGS.test} className="btn btn-line btn-sm" onClick={test} disabled={busy === "test"}>
                        <PlugZap size={14} /> {busy === "test" ? "Testing…" : "Test connection"}
                    </button>
                </div>
            </div>
        </div>
    );
}
