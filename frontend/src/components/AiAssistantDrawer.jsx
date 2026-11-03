import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { Sparkles, ShieldCheck, Send } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { AI } from "@/constants/testIds";
import { AI_DISABLED, aiErrorMessage, explainAi, fetchAiStatus } from "@/lib/ai";

const AiContext = createContext({ status: AI_DISABLED, enabled: false, explain: () => {} });

/** Hook for pages: `const { enabled, explain } = useAiAssistant();` */
export function useAiAssistant() {
    return useContext(AiContext);
}

/**
 * Provides AI status to the app and renders the right-hand assistant drawer.
 * Everything is hidden (enabled=false) unless an admin enabled the assistant in Settings.
 */
export function AiAssistantProvider({ children }) {
    const { pathname } = useLocation();
    const [status, setStatus] = useState(AI_DISABLED);
    const [target, setTarget] = useState(null);

    // Re-check on navigation so a Settings change shows up without a reload.
    useEffect(() => {
        let alive = true;
        fetchAiStatus().then((s) => alive && setStatus(s));
        return () => {
            alive = false;
        };
    }, [pathname]);

    const explain = useCallback((t) => setTarget(t), []);
    const value = useMemo(() => ({ status, enabled: !!status.enabled, explain }), [status, explain]);

    return (
        <AiContext.Provider value={value}>
            {children}
            {status.enabled && <AiDrawer status={status} target={target} onClose={() => setTarget(null)} />}
        </AiContext.Provider>
    );
}

/** Small button used on the pod diagnosis banner and on Warning events. Renders nothing when AI is off. */
export function ExplainWithAiButton({ target, testId, className = "" }) {
    const { t } = useTranslation();
    const { enabled, explain } = useAiAssistant();
    if (!enabled) return null;
    return (
        <Button
            type="button"
            variant="outline"
            size="sm"
            data-testid={testId}
            className={`rounded-full ${className}`}
            onClick={() => explain(target)}
        >
            <Sparkles className="h-3.5 w-3.5" /> {t("pages:aiAssistant.explain")}
        </Button>
    );
}

function AiDrawer({ status, target, onClose }) {
    const { t } = useTranslation();
    const open = !!target;
    const [question, setQuestion] = useState("");
    const [preview, setPreview] = useState(null);
    const [answer, setAnswer] = useState("");
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(false);
    const [sending, setSending] = useState(false);
    const seq = useRef(0);

    const targetKey = target ? `${target.namespace}|${target.kind}|${target.name}|${target.reason || ""}` : "";

    // New target: reset and load the redacted preview. Nothing is sent to the model yet.
    useEffect(() => {
        setQuestion("");
        setAnswer("");
        setError("");
        setPreview(null);
    }, [targetKey]);

    // Preview (re)loads when the target or the question changes (debounced for typing).
    useEffect(() => {
        if (!open) return undefined;
        const mine = ++seq.current;
        setLoading(true);
        const timer = setTimeout(
            () => {
                explainAi({ ...target, question, preview: true })
                    .then((r) => {
                        if (seq.current !== mine) return;
                        setPreview(r);
                        setError("");
                    })
                    .catch((e) => {
                        if (seq.current !== mine) return;
                        setPreview(null);
                        setError(aiErrorMessage(e));
                    })
                    .finally(() => seq.current === mine && setLoading(false));
            },
            question ? 500 : 0
        );
        return () => clearTimeout(timer);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, targetKey, question]);

    const send = async () => {
        setSending(true);
        setError("");
        setAnswer("");
        try {
            const r = await explainAi({ ...target, question, preview: false });
            setAnswer(r.answer || "");
        } catch (e) {
            setError(aiErrorMessage(e));
        } finally {
            setSending(false);
        }
    };

    const dest = preview?.destination;
    const where = dest ? `${dest.provider} (${dest.baseUrl})` : `${status.provider} (${status.baseUrl})`;

    return (
        <Sheet open={open} onOpenChange={(v) => !v && onClose()}>
            <SheetContent
                side="right"
                data-testid={AI.drawer}
                className="flex w-full flex-col gap-4 sm:max-w-xl"
            >
                <SheetHeader>
                    <SheetTitle className="flex items-center gap-2">
                        <Sparkles className="h-4 w-4" /> {t("pages:aiAssistant.explain")}
                    </SheetTitle>
                    <SheetDescription>
                        {target?.label || (target ? `${target.namespace}/${target.name}` : "")}
                    </SheetDescription>
                </SheetHeader>

                <div tabIndex={0} role="region" aria-label={t("pages:aiAssistant.detailsAria")} className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto pr-1">
                <div
                    data-testid={AI.notice}
                    role="note"
                    className="flex items-start gap-2 rounded-2xl border border-soft-info-border bg-soft-info px-3 py-2.5 text-xs text-muted-foreground"
                >
                    <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-tone-link" />
                    <div>
                        <div className="text-foreground">
                            <Trans
                                i18nKey="pages:aiAssistant.sentTo"
                                values={{ where }}
                                components={{ mono: <span className="mono font-medium" /> }}
                            />
                        </div>
                        <div className="mt-0.5">
                            {status.demo
                                ? t("pages:aiAssistant.demoNotice")
                                : status.redact
                                ? t("pages:aiAssistant.redactOn")
                                : t("pages:aiAssistant.redactOff")}
                        </div>
                    </div>
                </div>

                <div className="space-y-1.5">
                    <label htmlFor="ai-question" className="text-xs text-muted-foreground">
                        {t("pages:aiAssistant.question")}
                    </label>
                    <textarea
                        id="ai-question"
                        data-testid={AI.question}
                        value={question}
                        maxLength={500}
                        onChange={(e) => setQuestion(e.target.value)}
                        placeholder={t("pages:aiAssistant.questionPlaceholder")}
                        className="min-h-[64px] w-full rounded-xl border border-border bg-muted/30 px-3 py-2 text-sm"
                    />
                </div>

                <div className="min-h-0 space-y-1.5">
                    <div className="text-xs text-muted-foreground">
                        {preview?.payload?.truncated ? t("pages:aiAssistant.previewTruncated") : t("pages:aiAssistant.preview")}
                    </div>
                    {loading && !preview ? (
                        <Skeleton data-testid={AI.loading} className="h-40 w-full rounded-xl bg-muted" />
                    ) : preview ? (
                        <pre
                            data-testid={AI.preview}
                            tabIndex={0}
                            aria-label={t("pages:aiAssistant.previewAria")}
                            className="mono max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-xl border border-border bg-muted/30 p-3 text-xs"
                        >
                            {preview.payload.prompt}
                        </pre>
                    ) : null}
                </div>

                <div className="flex items-center gap-2">
                    <Button type="button" data-testid={AI.send} onClick={send} disabled={!preview || sending || loading}>
                        <Send className="h-3.5 w-3.5" /> {sending ? t("pages:aiAssistant.asking") : t("pages:aiAssistant.send")}
                    </Button>
                </div>

                {error && (
                    <div data-testid={AI.error} role="alert" className="text-sm text-tone-red">
                        {error}
                    </div>
                )}
                {answer && (
                    <div className="space-y-1.5" aria-live="polite">
                        <div className="text-xs text-muted-foreground">{t("pages:aiAssistant.answer")}</div>
                        {/* Rendered as a text node only: model output is never interpreted as HTML or Markdown. */}
                        <pre
                            data-testid={AI.answer}
                            tabIndex={0}
                            className="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-xl border border-border p-3 text-sm font-sans"
                        >
                            {answer}
                        </pre>
                    </div>
                )}
                </div>
            </SheetContent>
        </Sheet>
    );
}

export default AiAssistantProvider;
