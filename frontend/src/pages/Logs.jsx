import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, streamPodLogs } from "@/lib/api";
import MacTerm from "@/components/MacTerm";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
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
            <div data-testid={LOGS.root} className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-[60vh] rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div data-testid={LOGS.root} className="page">
            <PageHeader
                title="Pod Logs"
                description="Live streaming terminal via chunked HTTP."
                tools={
                    <>
                        <span className="flex items-center gap-1.5 text-[13.5px] text-ink-2">
                            <TerminalSquare size={15} className="text-ink-3" /> Pod
                        </span>
                        <select
                            data-testid={LOGS.podSelect}
                            aria-label="Pod"
                            className="select"
                            value={selected}
                            onChange={(e) => {
                                stop();
                                setSelected(e.target.value);
                            }}
                        >
                            {podOptions.length === 0 && <option value="">Choose pod</option>}
                            {podOptions.map((p) => (
                                <option key={p.value} value={p.value}>
                                    {p.label}
                                </option>
                            ))}
                        </select>
                        {containerOptions.length > 1 && (
                            <select aria-label="Container" className="select" value={container} onChange={(e) => setContainer(e.target.value)}>
                                {containerOptions.map((c) => (
                                    <option key={c} value={c}>
                                        {c}
                                    </option>
                                ))}
                            </select>
                        )}
                        {running ? (
                            <button type="button" data-testid={LOGS.stopBtn} className="btn btn-line" onClick={stop}>
                                <Square size={15} /> Stop
                            </button>
                        ) : (
                            <button type="button" data-testid={LOGS.startBtn} className="btn btn-fill" onClick={start}>
                                <Play size={15} /> Follow
                            </button>
                        )}
                        <span className="sp" />
                        <button type="button" data-testid={LOGS.clearBtn} className="btn btn-line" onClick={() => setLines([])}>
                            <Trash2 size={15} /> Clear
                        </button>
                    </>
                }
            />

            <MacTerm
                title={running ? `logs — ${selected}` : `logs — idle`}
                actions={
                    <span className="mono text-[11px] text-term-dim">
                        <span className={`status-dot mr-1.5 inline-block align-middle ${running ? "ok animate-pulse" : "info"}`} />
                        {lines.length} lines
                    </span>
                }
                bodyRef={listRef}
                bodyProps={{ "data-testid": LOGS.stream }}
                bodyClassName="h-[60vh] overflow-auto scrollbar-thin whitespace-pre-wrap px-4 py-3 text-[12.5px] leading-[1.55]"
            >
                {lines.length === 0 && (
                    <div className="text-term-dim">
                        Press <span className="text-term-key">Follow</span> to stream{" "}
                        <span className="text-term-str">{selected}</span>.
                    </div>
                )}
                {lines.map((l, i) => (
                    <div key={i} className="text-term-plain">
                        {l}
                    </div>
                ))}
            </MacTerm>
        </div>
    );
}
