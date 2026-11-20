import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@fontsource-variable/inter/wght.css"; // UI fallback off Apple platforms (SF Pro is preferred; this is only fetched when it is missing)
import "@fontsource-variable/jetbrains-mono/wght.css"; // mono fallback off Apple platforms
import "@/styles/tokens.css";
import "@/index.css";
import App from "@/App";
import { initLanguage } from "@/lib/i18n";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    },
  },
});

const root = ReactDOM.createRoot(document.getElementById("root"));
// Resolve the language (and fetch its chunk, unless it is English) before the
// first paint so the UI never flashes English for a German/Spanish/French user.
initLanguage().finally(() => root.render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </React.StrictMode>,
));
