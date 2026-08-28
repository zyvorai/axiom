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

const SHELLS = ["/bin/bash", "/bin/sh"];

export default function PodTerminal({ namespace, name, containers }) {
    const [container, setContainer] = useState(containers[0] || "");
    const [shell, setShell] = useState(SHELLS[0]);
    const [status, setStatus] = useState("idle"); // idle | connecting | connected | closed | error
    const containerRef = useRef(null);
    const termRef = useRef(null);
    const fitRef = useRef(null);
    const wsRef = useRef(null);

    useEffect(() => {
        const term = new XTerm({
            convertEol: true,
            fontFamily: "'JetBrains Mono', 'SF Mono', Menlo, monospace",
            fontSize: 13,
            theme: { background: "#00000000" },
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
            wsRef.current?.close();
            term.dispose();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const connect = () => {
        const term = termRef.current;
        term.reset();
        setStatus("connecting");
        const token = sessionStorage.getItem("axiom.basicAuth");
        const proto = window.location.protocol === "https:" ? "wss" : "ws";
        const params = new URLSearchParams({ container, shell });
        if (token) params.set("access_token", token);
        const ws = new WebSocket(`${proto}://${window.location.host}/api/v1/pods/${namespace}/${encodeURIComponent(name)}/terminal?${params.toString()}`);
        ws.binaryType = "arraybuffer";
        wsRef.current = ws;

        ws.onopen = () => {
            setStatus("connected");
            const { cols, rows } = term;
            ws.send(`__resize__:${cols}x${rows}`);
            term.onData((data) => ws.readyState === WebSocket.OPEN && ws.send(data));
        };
        ws.onmessage = (e) => {
            if (e.data instanceof ArrayBuffer) {
                term.write(new Uint8Array(e.data));
            } else {
                term.write(e.data);
            }
        };
        ws.onerror = () => setStatus("error");
        ws.onclose = () => setStatus("closed");
    };

    const disconnect = () => {
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
                        <SelectTrigger className="w-40 border-white/10 bg-white/[0.03]">
                            <SelectValue />
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
                    <SelectTrigger className="w-32 border-white/10 bg-white/[0.03]">
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
                    <Button size="sm" variant="outline" onClick={disconnect} className="rounded-xl border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20">
                        <Plug className="mr-1.5 h-3.5 w-3.5" /> Disconnect
                    </Button>
                ) : (
                    <Button size="sm" onClick={connect} disabled={status === "connecting"} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        <PlugZap className="mr-1.5 h-3.5 w-3.5" /> {status === "connecting" ? "Connecting…" : "Connect"}
                    </Button>
                )}
                <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span className={`status-dot ${status === "connected" ? "ok animate-pulse" : status === "error" ? "err" : "info"}`} />
                    {status}
                </span>
            </div>
            <div ref={containerRef} className="h-[420px] overflow-hidden rounded-lg border border-white/10 bg-black/60 p-2" />
        </div>
    );
}
