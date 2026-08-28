import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, streamPodLogs } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Play, Square, Trash2, TerminalSquare } from "lucide-react";
import { LOGS } from "@/constants/testIds";

export default function Logs() {
    const [searchParams] = useSearchParams();
    const [pods, setPods] = useState(null);
    const [selected, setSelected] = useState(searchParams.get("pod") || "");
    const [container, setContainer] = useState("");
    const [running, setRunning] = useState(false);
    const [lines, setLines] = useState([]);
    const controllerRef = useRef(null);
    const listRef = useRef(null);

    useEffect(() => {
        let alive = true;
        api.get("/pods").then((r) => {
            if (!alive) return;
            setPods(r.data);
            const fromUrl = searchParams.get("pod");
            if (fromUrl && r.data.some((p) => `${p.namespace}/${p.name}` === fromUrl)) {
                setSelected(fromUrl);
            } else if (!fromUrl && r.data[0]) {
                setSelected(`${r.data[0].namespace}/${r.data[0].name}`);
            }
        });
        return () => {
            alive = false;
            stop();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useEffect(() => {
        if (listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight;
    }, [lines]);

    const selectedPod = useMemo(() => (pods || []).find((p) => `${p.namespace}/${p.name}` === selected), [pods, selected]);
    const containerOptions = selectedPod?.containers || [];

    useEffect(() => {
        setContainer(containerOptions[0] || "");
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selected]);

    const start = () => {
        if (!selected || running) return;
        setLines([]);
        const [namespace, name] = selected.split("/");
        controllerRef.current = streamPodLogs(namespace, name, {
            container: container || undefined,
            onLine: (line) => setLines((prev) => (prev.length > 500 ? [...prev.slice(-500), line] : [...prev, line])),
            onError: () => setRunning(false),
            onEnd: () => setRunning(false),
        });
        setRunning(true);
    };

    const stop = () => {
        controllerRef.current?.abort();
        controllerRef.current = null;
        setRunning(false);
    };

    const podOptions = useMemo(
        () => (pods || []).map((p) => ({ value: `${p.namespace}/${p.name}`, label: `${p.namespace}/${p.name}` })),
        [pods]
    );

    if (!pods) {
        return (
            <div data-testid={LOGS.root} className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-[60vh] rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div data-testid={LOGS.root} className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Pod Logs</h1>
                    <p className="text-sm text-muted-foreground">Live streaming terminal via chunked HTTP.</p>
                </div>
            </div>

            <GlassCard className="flex flex-wrap items-center gap-3 p-3">
                <div className="flex items-center gap-2">
                    <TerminalSquare className="h-4 w-4 text-[#ff7550]" />
                    <span className="text-sm font-medium">Pod</span>
                </div>
                <Select
                    value={selected}
                    onValueChange={(v) => {
                        stop();
                        setSelected(v);
                    }}
                >
                    <SelectTrigger data-testid={LOGS.podSelect} className="w-[300px] border-white/10 bg-white/[0.03]">
                        <SelectValue placeholder="Choose pod" />
                    </SelectTrigger>
                    <SelectContent>
                        {podOptions.map((p) => (
                            <SelectItem key={p.value} value={p.value}>
                                {p.label}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>
                {containerOptions.length > 1 && (
                    <Select value={container} onValueChange={setContainer}>
                        <SelectTrigger className="w-[160px] border-white/10 bg-white/[0.03]">
                            <SelectValue placeholder="Container" />
                        </SelectTrigger>
                        <SelectContent>
                            {containerOptions.map((c) => (
                                <SelectItem key={c} value={c}>
                                    {c}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                )}
                {running ? (
                    <Button
                        data-testid={LOGS.stopBtn}
                        onClick={stop}
                        variant="outline"
                        className="rounded-xl border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                    >
                        <Square className="mr-1.5 h-4 w-4" /> Stop
                    </Button>
                ) : (
                    <Button
                        data-testid={LOGS.startBtn}
                        onClick={start}
                        className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white hover:brightness-110"
                    >
                        <Play className="mr-1.5 h-4 w-4" /> Follow
                    </Button>
                )}
                <Button
                    data-testid={LOGS.clearBtn}
                    variant="ghost"
                    onClick={() => setLines([])}
                    className="ml-auto rounded-xl text-muted-foreground hover:text-foreground"
                >
                    <Trash2 className="mr-1.5 h-4 w-4" /> Clear
                </Button>
            </GlassCard>

            <GlassCard className="p-0">
                <div className="flex items-center justify-between border-b border-white/5 px-4 py-2">
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span className={`status-dot ${running ? "ok animate-pulse" : "info"}`} />
                        <span className="mono">{running ? `streaming ${selected}` : "idle"}</span>
                    </div>
                    <div className="mono text-[11px] text-muted-foreground">{lines.length} lines</div>
                </div>
                <div
                    ref={listRef}
                    data-testid={LOGS.stream}
                    className="mono h-[60vh] overflow-auto scrollbar-thin whitespace-pre-wrap bg-black/40 p-4 text-[12.5px] leading-relaxed"
                >
                    {lines.length === 0 && (
                        <div className="text-muted-foreground">
                            Press <span className="text-foreground">Follow</span> to start streaming logs for{" "}
                            <span className="text-[#ff7550]">{selected}</span>.
                        </div>
                    )}
                    {lines.map((l, i) => (
                        <div key={i} className="text-foreground/90">
                            {l}
                        </div>
                    ))}
                </div>
            </GlassCard>
        </div>
    );
}
