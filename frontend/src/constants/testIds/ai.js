// Test IDs for the opt-in AI assistant (Settings card, "Explain with AI" buttons, drawer).
// Naming follows the directive in ./auth.js (keys camelCase, values kebab-case).

export const AI = {
    explainPod: 'ai-explain-pod',
    explainEvent: (i) => `ai-explain-event-${i}`,
    explainFinding: 'ai-explain-finding',
    drawer: 'ai-drawer',
    notice: 'ai-send-notice',
    preview: 'ai-payload-preview',
    question: 'ai-question',
    send: 'ai-send',
    answer: 'ai-answer',
    error: 'ai-error',
    loading: 'ai-loading',
};

export const AI_SETTINGS = {
    card: 'ai-settings-card',
    enabled: 'ai-settings-enabled',
    provider: 'ai-settings-provider',
    baseUrl: 'ai-settings-base-url',
    model: 'ai-settings-model',
    apiKey: 'ai-settings-api-key',
    redact: 'ai-settings-redact',
    save: 'ai-settings-save',
    test: 'ai-settings-test',
};
