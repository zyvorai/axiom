import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, streamPodLogs, streamWorkloadLogs } from "@/lib/api";
import MacTerm from "@/components/MacTerm";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Download, Filter, Play, Square, Trash2, TerminalSquare } from "lucide-react";
import { LOGS, WLOGS } from "@/constants/testIds";
import { compileFilter, parseLogLine } from "@/lib/troubleshoot";

const MAX_LINES = 2000;
const WORKLOAD_KINDS = new Set(["Deployment", "StatefulSet", "DaemonSet", "Job"]);

export default function Logs() {
    const [searchParams] = useSearchParams();
    const initialWorkload = searchParams.get("workload") || "";
    const [mode, setMode] = useState(initialWorkload ? "workload" : "pod");
    const [pods, setPods] = useState(null);
    const [workloads, setWorkloads] = useState(null);
    const [selected, setSelected] = useState(searchParams.get("pod") || "");
    const [selectedWorkload, setSelectedWorkload] = useState(initialWorkload);
    const [container, setContainer] = useState("");
    const [running, setRunning] = useState(false);
    const [lines, setLines] = useState([]);
    const [notice, setNotice] = useState("");
    const [filter, setFilter] = useState("");
    const [showTimestamps, setShowTimestamps] = useState(true);
    const [previous, setPrevious] = useState(false);
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

    // Workloads are only needed for workload mode; fetch them the first time it is used.
    useEffect(() => {
        if (mode !== "workload" || workloads) return undefined;
        let alive = true;
        api
            .get("/workloads")
            .then((r) => {
                if (!alive) return;
                const rows = (r.data || []).filter((w) => WORKLOAD_KINDS.has(w.kind));
                setWorkloads(rows);
                setSelectedWorkload((cur) => cur || (rows[0] ? `${rows[0].namespace}/${rows[0].kind}/${rows[0].name}` : ""));
            })
            .catch(() => alive && setWorkloads([]));
        return () => {
            alive = false;
        };
    }, [mode, workloads]);

    const selectedPod = useMemo(() => (pods || []).find((p) => `${p.namespace}/${p.name}` === selected), [pods, selected]);
    const containerOptions = selectedPod?.containers || [];

    useEffect(() => {
        setContainer(containerOptions[0] || "");
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selected]);

    const { re, error: filterError } = useMemo(() => compileFilter(filter), [filter]);
    const visible = useMemo(() => {
        const parsed = lines.map(parseLogLine);
        return re ? parsed.filter((l) => re.test(`${l.pod} ${l.text}`)) : parsed;
    }, [lines, re]);

    useEffect(() => {
        if (listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight;
    }, [visible]);

    const target = mode === "workload" ? selectedWorkload : selected;

    const start = () => {
        if (!target || running) return;
        setLines([]);
        setNotice("");
        const push = (line) => setLines((prev) => (prev.length >= MAX_LINES ? [...prev.slice(-(MAX_LINES - 1)), line] : [...prev, line]));
        const handlers = {
            onLine: push,
            onError: (e) => {
                setNotice(e?.message || "The log stream failed.");
                setRunning(false);
            },
            onEnd: () => setRunning(false),
        };
        if (mode === "workload") {
            const [namespace, kind, name] = selectedWorkload.split("/");
            // The previous container's logs are a finished snapshot, not a live stream.
            controllerRef.current = streamWorkloadLogs(namespace, kind, name, { follow: !previous, previous, ...handlers });
        } else {
            const [namespace, name] = selected.split("/");
            controllerRef.current = streamPodLogs(namespace, name, { container: container || undefined, follow: !previous, previous, ...handlers });
        }
        setRunning(true);
    };

    const stop = () => {
        controllerRef.current?.abort();
        controllerRef.current = null;
        setRunning(false);
    };

    const switchMode = (next) => {
        if (next === mode) return;
        stop();
        setLines([]);
        setNotice("");
        setMode(next);
    };

    const download = () => {
        const text = visible.map((l) => `${l.pod ? `[${l.pod}] ` : ""}${l.ts ? `${l.ts} ` : ""}${l.text}`).join("\n") + "\n";
        const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
        const a = document.createElement("a");
        a.href = url;
        a.download = `logs-${(target || "output").replace(/\//g, "_")}.txt`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    };

    const podOptions = useMemo(
        () => (pods || []).map((p) => ({ value: `${p.namespace}/${p.name}`, label: `${p.namespace}/${p.name}` })),
        [pods]
    );
    const workloadOptions = useMemo(
        () => (workloads || []).map((w) => ({ value: `${w.namespace}/${w.kind}/${w.name}`, label: `${w.namespace}/${w.kind}/${w.name}` })),
        [workloads]
    );

    if (!pods) {
        return (
            <div data-testid={LOGS.root} className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-[60vh] rounded-2xl bg-muted" />
            </div>
        );
    }

    const shown = visible.slice(-MAX_LINES);

    return (
        <div data-testid={LOGS.root} className="page">
            <PageHeader
                title={mode === "workload" ? "Workload Logs" : "Pod Logs"}
                description={
                    mode === "workload"
                        ? "Every pod and container of a workload, merged into one stream and prefixed with [pod/container]."
                        : "Live streaming terminal via chunked HTTP."
                }
                tools={
                    <>
                        <div className="seg" role="group" aria-label="Log source">
                            <button type="button" data-testid={WLOGS.modePod} aria-pressed={mode === "pod"} onClick={() => switchMode("pod")}>
                                Pod
                            </button>
                            <button type="button" data-testid={WLOGS.modeWorkload} aria-pressed={mode === "workload"} onClick={() => switchMode("workload")}>
                                Workload
                            </button>
                        </div>
                        {mode === "pod" ? (
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
                            </>
                        ) : (
                            <select
                                data-testid={WLOGS.workloadSelect}
                                aria-label="Workload"
                                className="select"
                                value={selectedWorkload}
                                onChange={(e) => {
                                    stop();
                                    setSelectedWorkload(e.target.value);
                                }}
                            >
                                {workloadOptions.length === 0 && <option value="">{workloads ? "No workloads" : "Loading…"}</option>}
                                {workloadOptions.map((w) => (
                                    <option key={w.value} value={w.value}>
                                        {w.label}
                                    </option>
                                ))}
                            </select>
                        )}
                        {running ? (
                            <button type="button" data-testid={LOGS.stopBtn} className="btn btn-line" onClick={stop}>
                                <Square size={15} /> Stop
                            </button>
                        ) : (
                            <button type="button" data-testid={LOGS.startBtn} className="btn btn-fill" onClick={start} disabled={!target}>
                                <Play size={15} /> {previous ? "Load" : "Follow"}
                            </button>
                        )}
                        <span className="sp" />
                        <button type="button" data-testid={LOGS.clearBtn} className="btn btn-line" onClick={() => setLines([])}>
                            <Trash2 size={15} /> Clear
                        </button>
                        <button type="button" data-testid={WLOGS.download} className="btn btn-line" onClick={download} disabled={visible.length === 0}>
                            <Download size={15} /> Download
                        </button>
                    </>
                }
            />

            <div className="tools" style={{ marginTop: 0 }}>
                <div className="input">
                    <Filter size={15} className="text-ink-3" aria-hidden="true" />
                    <input
                        data-testid={WLOGS.filter}
                        aria-label="Filter lines (regular expression)"
                        aria-invalid={filterError ? "true" : "false"}
                        placeholder="Filter lines (regex, case-insensitive)"
                        value={filter}
                        onChange={(e) => setFilter(e.target.value)}
                        className="bare-input mono"
                    />
                </div>
                <label className="flex items-center gap-1.5 text-[13.5px] text-ink-2">
                    <input
                        type="checkbox"
                        data-testid={WLOGS.timestamps}
                        checked={showTimestamps}
                        onChange={(e) => setShowTimestamps(e.target.checked)}
                    />
                    Timestamps
                </label>
                <label className="flex items-center gap-1.5 text-[13.5px] text-ink-2">
                    <input
                        type="checkbox"
                        data-testid={WLOGS.previous}
                        checked={previous}
                        disabled={running}
                        onChange={(e) => setPrevious(e.target.checked)}
                    />
                    Previous container
                </label>
            </div>
            {filterError && (
                <p role="alert" data-testid={WLOGS.filterError} className="text-[13px] text-tone-red">
                    Invalid regular expression: {filterError}
                </p>
            )}
            {notice && (
                <div className="notice danger" role="alert">
                    {notice}
                </div>
            )}

            <MacTerm
                title={running ? `logs — ${target}` : `logs — idle`}
                actions={
                    <span className="mono text-[11px] text-term-dim" data-testid={WLOGS.count}>
                        <span className={`status-dot mr-1.5 inline-block align-middle ${running ? "ok animate-pulse" : "info"}`} />
                        {re ? `${visible.length}/${lines.length}` : lines.length} lines
                    </span>
                }
                bodyRef={listRef}
                bodyProps={{ "data-testid": LOGS.stream }}
                bodyClassName="h-[60vh] overflow-auto scrollbar-thin whitespace-pre-wrap px-4 py-3 text-[12.5px] leading-[1.55]"
            >
                {lines.length === 0 && (
                    <div className="text-term-dim">
                        Press <span className="text-term-key">{previous ? "Load" : "Follow"}</span> to {previous ? "read the previous run of" : "stream"}{" "}
                        <span className="text-term-str">{target || "a target"}</span>.
                    </div>
                )}
                {shown.map((l, i) => (
                    <div key={i} className="text-term-plain" data-testid={WLOGS.line(i)}>
                        {l.pod && <span className="text-term-key">[{l.pod}] </span>}
                        {showTimestamps && l.ts && <span className="text-term-dim">{l.ts} </span>}
                        {l.text}
                    </div>
                ))}
            </MacTerm>
        </div>
    );
}
