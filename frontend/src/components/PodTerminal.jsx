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

const SHELLS = ["/bin/sh", "/bin/bash", "/bin/ash"];

export default function PodTerminal({ namespace, name, containers }) {
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
            fontFamily: "'JetBrains Mono', 'SF Mono', Menlo, monospace",
            fontSize: 13,
            theme: {
                background: "#0d1117",
                foreground: "#e6edf3",
                cursor: "#2f81f7",
            },
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

    const connect = () => {
        const term = termRef.current;
        if (!term) return;
        dataDispRef.current?.dispose();
        wsRef.current?.close();
        term.reset();
        term.writeln(`\x1b[90mConnecting to ${namespace}/${name} (${shell})…\x1b[0m`);
        setStatus("connecting");
        const token = sessionStorage.getItem("axiom.basicAuth");
        const proto = window.location.protocol === "https:" ? "wss" : "ws";
        const params = new URLSearchParams({ shell });
        if (container) params.set("container", container);
        if (token) params.set("access_token", token);
        const ws = new WebSocket(
            `${proto}://${window.location.host}/api/v1/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/terminal?${params.toString()}`
        );
        ws.binaryType = "arraybuffer";
        wsRef.current = ws;

        ws.onopen = () => {
            setStatus("connected");
            const { cols, rows } = term;
            ws.send(`__resize__:${cols}x${rows}`);
            dataDispRef.current = term.onData((data) => {
                if (ws.readyState === WebSocket.OPEN) ws.send(data);
            });
        };
        ws.onmessage = (e) => {
            if (e.data instanceof ArrayBuffer) {
                term.write(new Uint8Array(e.data));
            } else {
                term.write(e.data);
            }
        };
        ws.onerror = () => {
            setStatus("error");
            term.writeln(
                "\r\n\x1b[31mWebSocket error.\x1b[0m If this site uses a self-signed cert, open the page over HTTPS once and accept the certificate, then retry Connect."
            );
        };
        ws.onclose = (ev) => {
            setStatus("closed");
            dataDispRef.current?.dispose();
            dataDispRef.current = null;
            if (ev.code !== 1000 && ev.code !== 1001) {
                const reason = ev.reason || `code ${ev.code}`;
                term.writeln(`\r\n\x1b[33mSession closed (${reason}).\x1b[0m`);
                term.writeln(
                    "\x1b[90mTip: pick /bin/sh (or ash). Distroless images have no shell — try a workload like redis/nginx.\x1b[0m"
                );
            }
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
                            <SelectValue placeholder="Container" />
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
                        <Plug className="mr-1.5 h-3.5 w-3.5" /> Disconnect
                    </Button>
                ) : (
                    <Button size="sm" variant="brand" onClick={connect} disabled={status === "connecting"} className="rounded-xl">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {status === "connecting" ? "Connecting…" : "Connect"}
                    </Button>
                )}
                <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span className={`status-dot ${status === "connected" ? "ok animate-pulse" : status === "error" ? "err" : "info"}`} />
                    {status}
                </span>
            </div>
            <div ref={containerRef} className="h-[420px] overflow-hidden rounded-lg border border-border bg-[#0d1117] p-2" />
        </div>
    );
}
