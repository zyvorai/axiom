import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Plug, PlugZap } from "lucide-react";
import MacTerm from "@/components/MacTerm";
import { buildTerminalTheme } from "@/lib/terminalTheme";
import { getStreamTicket } from "@/lib/api";
import { useTranslation, Trans } from "react-i18next";

const SHELLS = ["/bin/sh", "/bin/bash", "/bin/ash"];

// Display labels for the connection status; the status value itself drives logic and styling.
const STATUS_KEYS = {
    idle: "pages:podTerminal.status.idle",
    connecting: "pages:podTerminal.status.connecting",
    connected: "pages:podTerminal.status.connected",
    closed: "pages:podTerminal.status.closed",
    error: "pages:podTerminal.status.error",
};

// `wsPath` overrides the WebSocket route (the node shell reuses this terminal);
// `label` overrides the "namespace/name" shown in the banner and window title.
export default function PodTerminal({ namespace, name, containers, wsPath, label }) {
    const { t } = useTranslation();
    const display = label || `${namespace}/${name}`;
    const [container, setContainer] = useState(containers[0] || "");
    const [shell, setShell] = useState(SHELLS[0]);
    const [status, setStatus] = useState("idle"); // idle | connecting | connected | closed | error
    const containerRef = useRef(null);
    const termRef = useRef(null);
    const fitRef = useRef(null);
    const wsRef = useRef(null);
    const dataDispRef = useRef(null);

    useEffect(() => {
        if (containers?.length && !containers.includes(container)) {
            setContainer(containers[0]);
        }
    }, [containers, container]);

    useEffect(() => {
        const term = new XTerm({
            convertEol: true,
            fontFamily: "'SF Mono', Menlo, Monaco, 'JetBrains Mono', monospace",
            fontSize: 13,
            theme: buildTerminalTheme(),
            cursorBlink: true,
        });
        const fit = new FitAddon();
        term.loadAddon(fit);
        term.open(containerRef.current);
        fit.fit();
        termRef.current = term;
        fitRef.current = fit;

        const onResize = () => fit.fit();
        window.addEventListener("resize", onResize);
        return () => {
            window.removeEventListener("resize", onResize);
            dataDispRef.current?.dispose();
            wsRef.current?.close();
            term.dispose();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const connect = async () => {
        const term = termRef.current;
        if (!term) return;
        dataDispRef.current?.dispose();
        wsRef.current?.close();
        term.reset();
        term.writeln(`\x1b[90m${t("pages:podTerminal.connectingTo", { target: display, shell })}\x1b[0m`);
        setStatus("connecting");

        let sawOpen = false;
        let sawPayload = false;
        let explained = false;

        // Credentials never go in the URL: trade the session for a single-use,
        // 30-second ticket bound to this exact terminal path.
        const path = wsPath || `/api/v1/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/terminal`;
        let ticket;
        try {
            ticket = await getStreamTicket(path.replace(/^\/api\/v1/, ""));
        } catch (e) {
            setStatus("error");
            term.writeln(`\r\n\x1b[31m${t("pages:podTerminal.ticketFailed", { error: e?.response?.data?.error || e.message })}\x1b[0m`);
            return;
        }
        const proto = window.location.protocol === "https:" ? "wss" : "ws";
        const params = new URLSearchParams({ shell, ticket });
        if (container) params.set("container", container);
        const ws = new WebSocket(`${proto}://${window.location.host}${path}?${params.toString()}`);
        ws.binaryType = "arraybuffer";
        wsRef.current = ws;

        const notePayload = (text) => {
            if (!text) return;
            sawPayload = true;
            const lower = text.toLowerCase();
            if (lower.includes("exec failed") || lower.includes("no such file") || lower.includes("distroless") || lower.includes("no shell")) {
                explained = true;
            }
        };

        ws.onopen = () => {
            sawOpen = true;
            setStatus("connected");
            const { cols, rows } = term;
            ws.send(`__resize__:${cols}x${rows}`);
            dataDispRef.current = term.onData((data) => {
                if (ws.readyState === WebSocket.OPEN) ws.send(data);
            });
        };
        ws.onmessage = (e) => {
            if (e.data instanceof ArrayBuffer) {
                const bytes = new Uint8Array(e.data);
                try {
                    notePayload(new TextDecoder().decode(bytes));
                } catch {
                    sawPayload = true;
                }
                term.write(bytes);
            } else {
                notePayload(String(e.data));
                term.write(e.data);
            }
        };
        ws.onerror = () => {
            // Browsers often fire onerror before onclose even when the server
            // already streamed an exec failure — don't blame TLS in that case.
            if (sawOpen || sawPayload || explained) return;
            setStatus("error");
            term.writeln(
                `\r\n\x1b[31m${t("pages:podTerminal.wsFailed")}\x1b[0m ${t("pages:podTerminal.wsFailedHint")}`
            );
        };
        ws.onclose = (ev) => {
            setStatus("closed");
            dataDispRef.current?.dispose();
            dataDispRef.current = null;
            if (ev.code === 1000 || ev.code === 1001) return;
            if (explained) return;
            if (!sawOpen && !sawPayload) {
                term.writeln(`\r\n\x1b[33m${t("pages:podTerminal.closedEarly")}\x1b[0m`);
                return;
            }
            term.writeln(`\r\n\x1b[33m${t("pages:podTerminal.sessionEnded")}\x1b[0m`);
            term.writeln(
                `\x1b[90m${t("pages:podTerminal.distrolessHint")}\x1b[0m`
            );
        };
    };

    const disconnect = () => {
        dataDispRef.current?.dispose();
        dataDispRef.current = null;
        wsRef.current?.close();
        wsRef.current = null;
        setStatus("closed");
    };

    useEffect(() => {
        if (status === "connected" && fitRef.current) {
            fitRef.current.fit();
            const { cols, rows } = termRef.current;
            wsRef.current?.readyState === WebSocket.OPEN && wsRef.current.send(`__resize__:${cols}x${rows}`);
        }
    }, [status]);

    return (
        <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
                {containers.length > 1 && (
                    <Select value={container} onValueChange={setContainer} disabled={status === "connected"}>
                        <SelectTrigger className="w-40">
                            <SelectValue placeholder={t("pages:podTerminal.container")} />
                        </SelectTrigger>
                        <SelectContent>
                            {containers.map((c) => (
                                <SelectItem key={c} value={c}>
                                    {c}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                )}
                <Select value={shell} onValueChange={setShell} disabled={status === "connected"}>
                    <SelectTrigger className="w-32">
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        {SHELLS.map((s) => (
                            <SelectItem key={s} value={s}>
                                {s}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>
                {status === "connected" ? (
                    <Button size="sm" variant="outline" onClick={disconnect} className="rounded-xl">
                        <Plug className="mr-1.5 h-3.5 w-3.5" /> {t("pages:podTerminal.disconnect")}
                    </Button>
                ) : (
                    <Button size="sm" variant="brand" onClick={connect} disabled={status === "connecting"} className="rounded-xl">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {status === "connecting" ? t("pages:podTerminal.connecting") : t("pages:podTerminal.connect")}
                    </Button>
                )}
                <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span className={`status-dot ${status === "connected" ? "ok animate-pulse" : status === "error" ? "err" : "info"}`} />
                    {t(STATUS_KEYS[status])}
                </span>
            </div>
            {(namespace === "axiom-system" || (name || "").startsWith("zyvor-axiom")) && (
                <p className="text-xs text-tone-amber dark:text-tone-amber">
                    <Trans i18nKey="pages:podTerminal.axiomDistroless" components={{ code: <span className="mono" /> }} />
                </p>
            )}
            <MacTerm
                title={`${display} — ${shell}`}
                actions={
                    <span className="mono text-[11px] text-term-dim">
                        <span className={`status-dot mr-1.5 inline-block align-middle ${status === "connected" ? "ok animate-pulse" : status === "error" ? "err" : "info"}`} />
                        {t(STATUS_KEYS[status])}
                    </span>
                }
                bodyClassName="!p-0 h-[420px] overflow-hidden"
            >
                <div ref={containerRef} className="h-full px-2 py-2" />
            </MacTerm>
        </div>
    );
}
