import { useCallback, useEffect, useMemo, useState } from "react";
import { Download, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useConfirm } from "@/components/ConfirmDialog";
import MacTerm from "@/components/MacTerm";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { HELM_CATALOG as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";

// One-click suggestions for the repositories most people want. Axiom never
// fetches any repository until an administrator adds it.
const SUGGESTED = [
    { name: "bitnami", url: "https://charts.bitnami.com/bitnami" },
    { name: "jetstack", url: "https://charts.jetstack.io" },
    { name: "ingress-nginx", url: "https://kubernetes.github.io/ingress-nginx" },
    { name: "prometheus-community", url: "https://prometheus-community.github.io/helm-charts" },
    { name: "grafana", url: "https://grafana.github.io/helm-charts" },
    { name: "argo", url: "https://argoproj.github.io/argo-helm" },
];

function RepoManager({ repos, onChanged, canWrite }) {
    const confirm = useConfirm();
    const [name, setName] = useState("");
    const [url, setUrl] = useState("");
    const [busy, setBusy] = useState(false);

    const add = async (n, u) => {
        setBusy(true);
        try {
            await api.post("/helm/repos", { name: n, url: u });
            toast.success(`Added repository ${n}`);
            setName("");
            setUrl("");
            onChanged();
        } catch (e) {
            toast.error("Could not add the repository", { description: errText(e) });
        } finally {
            setBusy(false);
        }
    };

    const remove = async (r) => {
        const ok = await confirm({
            title: `Remove repository ${r.name}?`,
            description: "Charts already installed from it keep running; they just stop appearing in the catalog.",
            confirmLabel: "Remove",
        });
        if (!ok) return;
        try {
            await api.delete(`/helm/repos/${encodeURIComponent(r.name)}`);
            toast.success(`Removed ${r.name}`);
            onChanged();
        } catch (e) {
            toast.error("Remove failed", { description: errText(e) });
        }
    };

    const known = new Set(repos.map((r) => r.name));
    return (
        <div className="card space-y-3 p-4">
            <div className="text-sm font-medium">Repositories</div>
            {repos.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                    No chart repositories yet. Axiom fetches a repository only after an administrator adds it.
                </p>
            ) : (
                <ul className="divide-y divide-border">
                    {repos.map((r) => (
                        <li key={r.name} className="flex items-center justify-between gap-3 py-2" data-testid={T.repoRow(r.name)}>
                            <div className="min-w-0">
                                <div className="text-sm font-medium">{r.name}</div>
                                <div className="mono truncate text-[11.5px] text-ink-3" title={r.url}>{r.url}</div>
                                {r.error ? <div className="text-xs text-tone-red">{r.error}</div> : null}
                            </div>
                            <div className="flex shrink-0 items-center gap-2 text-xs text-ink-3">
                                {r.chartCount ? <span>{r.chartCount} charts</span> : null}
                                {canWrite && (
                                    <button type="button" className="btn btn-danger btn-sm" data-testid={T.repoRemove(r.name)} aria-label={`Remove repository ${r.name}`} onClick={() => remove(r)}>
                                        <Trash2 size={13} />
                                    </button>
                                )}
                            </div>
                        </li>
                    ))}
                </ul>
            )}
            {canWrite && (
                <>
                    <form
                        className="grid gap-2 sm:grid-cols-[10rem_1fr_auto]"
                        onSubmit={(e) => {
                            e.preventDefault();
                            if (name && url) add(name.trim(), url.trim());
                        }}
                    >
                        <div>
                            <Label htmlFor="helm-repo-name" className="sr-only">Repository name</Label>
                            <Input id="helm-repo-name" data-testid={T.repoName} value={name} onChange={(e) => setName(e.target.value)} placeholder="name" className="bg-muted/30" />
                        </div>
                        <div>
                            <Label htmlFor="helm-repo-url" className="sr-only">Repository URL</Label>
                            <Input id="helm-repo-url" data-testid={T.repoUrl} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://charts.example.com" className="bg-muted/30" />
                        </div>
                        <button type="submit" className="btn btn-fill btn-sm" data-testid={T.repoAdd} disabled={busy || !name || !url}>
                            <Plus size={14} /> {busy ? "Adding…" : "Add"}
                        </button>
                    </form>
                    <div className="flex flex-wrap items-center gap-1.5 text-xs text-ink-3">
                        <span>Suggested:</span>
                        {SUGGESTED.filter((s) => !known.has(s.name)).map((s) => (
                            <button key={s.name} type="button" className="pill soft" data-testid={T.repoSuggest(s.name)} disabled={busy} onClick={() => add(s.name, s.url)}>
                                + {s.name}
                            </button>
                        ))}
                    </div>
                </>
            )}
        </div>
    );
}

function ChartDialog({ chart, repo, onClose, onInstalled }) {
    const [detail, setDetail] = useState(null);
    const [version, setVersion] = useState("");
    const [vals, setVals] = useState(null); // {values, readme}
    const [valsError, setValsError] = useState("");
    const [loadingVals, setLoadingVals] = useState(false);
    const [showReadme, setShowReadme] = useState(false);
    const [namespace, setNamespace] = useState("default");
    const [release, setRelease] = useState("");
    const [values, setValues] = useState("");
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (!chart) return;
        setDetail(null);
        setVals(null);
        setValsError("");
        setShowReadme(false);
        setNamespace("default");
        setRelease(chart.name);
        setValues("");
        setVersion(chart.version);
        api
            .get(`/helm/charts/${encodeURIComponent(chart.repo)}/${encodeURIComponent(chart.name)}`)
            .then((r) => setDetail(r.data))
            .catch((e) => setValsError(errText(e)));
    }, [chart]);

    const loadValues = async () => {
        setLoadingVals(true);
        setValsError("");
        try {
            const { data } = await api.get(
                `/helm/charts/${encodeURIComponent(chart.repo)}/${encodeURIComponent(chart.name)}/values`,
                { params: { version } }
            );
            setVals(data);
            // Offer the defaults as the starting point of the editable values.
            setValues((cur) => cur || data.values || "");
        } catch (e) {
            setValsError(errText(e));
        } finally {
            setLoadingVals(false);
        }
    };

    const install = async () => {
        setBusy(true);
        try {
            await api.post("/helm/install", {
                namespace,
                releaseName: release,
                repoUrl: repo.url,
                chartName: chart.name,
                chartVersion: version,
                values,
            });
            toast.success(`Install started for ${release}`, { description: "It runs as a background Job; the release appears under Releases once Helm finishes." });
            onInstalled();
            onClose();
        } catch (e) {
            toast.error("Install failed", { description: errText(e) });
        } finally {
            setBusy(false);
        }
    };

    if (!chart) return null;
    const versions = detail?.versions || [];
    return (
        <Dialog open={!!chart} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-2xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>{chart.name}</DialogTitle>
                    <DialogDescription>
                        {chart.description || "No description."} · from <span className="mono">{chart.repo}</span>
                    </DialogDescription>
                </DialogHeader>

                <div tabIndex={0} role="region" aria-label="Chart install options" className="max-h-[65vh] space-y-4 overflow-y-auto pr-1">
                <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5">
                        <Label htmlFor="helm-chart-version" className="text-xs text-muted-foreground">Version</Label>
                        <select
                            id="helm-chart-version"
                            data-testid={T.versionSelect}
                            className="select w-full"
                            value={version}
                            onChange={(e) => {
                                setVersion(e.target.value);
                                setVals(null);
                            }}
                        >
                            {(versions.length ? versions : [{ version: chart.version }]).map((v) => (
                                <option key={v.version} value={v.version}>
                                    {v.version}
                                    {v.appVersion ? ` (app ${v.appVersion})` : ""}
                                    {v.deprecated ? " — deprecated" : ""}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="helm-chart-namespace" className="text-xs text-muted-foreground">Namespace</Label>
                        <Input id="helm-chart-namespace" data-testid={T.namespace} value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5 sm:col-span-2">
                        <Label htmlFor="helm-chart-release" className="text-xs text-muted-foreground">Release name</Label>
                        <Input id="helm-chart-release" data-testid={T.release} value={release} onChange={(e) => setRelease(e.target.value)} className="bg-muted/30" />
                    </div>
                </div>

                <div className="space-y-2">
                    <div className="flex items-center justify-between">
                        <span className="text-xs text-muted-foreground">Values (optional)</span>
                        <button type="button" className="btn btn-line btn-sm" data-testid={T.showValues} onClick={loadValues} disabled={loadingVals}>
                            <Download size={13} /> {loadingVals ? "Loading…" : vals ? "Reload defaults" : "Load default values"}
                        </button>
                    </div>
                    {valsError ? <p className="text-xs text-tone-red" role="alert">{valsError}</p> : null}
                    <MacTerm title="values.yaml" bodyClassName="!p-0">
                        <Textarea
                            aria-label="Values YAML"
                            data-testid={T.valuesEditor}
                            value={values}
                            onChange={(e) => setValues(e.target.value)}
                            placeholder="# leave empty for the chart defaults"
                            className="macos-term-body mono min-h-[160px] resize-y rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0"
                            spellCheck={false}
                        />
                    </MacTerm>
                    {vals?.readme ? (
                        <div>
                            <button type="button" className="text-xs text-tone-link underline" onClick={() => setShowReadme((v) => !v)}>
                                {showReadme ? "Hide README" : "Show README"}
                            </button>
                            {showReadme ? (
                                <pre tabIndex={0} role="region" aria-label="Chart README" className="mono mt-2 max-h-64 overflow-auto rounded-xl bg-muted p-3 text-xs" data-testid={T.readme}>{vals.readme}</pre>
                            ) : null}
                        </div>
                    ) : null}
                    <p className="text-[11px] text-muted-foreground">
                        Runs as a background Job using Axiom&apos;s own permissions — the chart can only create resources Axiom itself is allowed to.
                    </p>
                </div>

                </div>

                <DialogFooter>
                    <button type="button" className="btn btn-fill" data-testid={T.install} onClick={install} disabled={busy || !release || !namespace}>
                        {busy ? "Starting…" : `Install ${version}`}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

/** Browse and install charts from administrator-approved Helm repositories. */
export default function HelmCatalog({ onInstalled }) {
    const { isViewer } = useAuth();
    const [repos, setRepos] = useState(null);
    const [repoFilter, setRepoFilter] = useState("");
    const [q, setQ] = useState("");
    const [result, setResult] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [selected, setSelected] = useState(null);

    const loadRepos = useCallback(async () => {
        try {
            const { data } = await api.get("/helm/repos");
            setRepos(data || []);
            setError("");
        } catch (e) {
            setError(errText(e));
            setRepos([]);
        }
    }, []);

    useEffect(() => {
        loadRepos();
    }, [loadRepos]);

    const search = useCallback(async () => {
        setLoading(true);
        try {
            const { data } = await api.get("/helm/charts", { params: { repo: repoFilter || undefined, q: q || undefined, limit: 60 } });
            setResult(data);
            setError("");
        } catch (e) {
            setError(errText(e));
        } finally {
            setLoading(false);
        }
    }, [repoFilter, q]);

    // Search as the person types, once there is at least one repository.
    useEffect(() => {
        if (!repos || repos.length === 0) {
            setResult(null);
            return undefined;
        }
        const t = setTimeout(search, 250);
        return () => clearTimeout(t);
    }, [repos, search]);

    const repoByName = useMemo(() => Object.fromEntries((repos || []).map((r) => [r.name, r])), [repos]);
    const errors = result?.errors ? Object.entries(result.errors) : [];

    if (!repos) return <Skeleton className="h-48 rounded-2xl bg-muted" />;

    return (
        <div className="stack" data-testid={T.root}>
            <RepoManager repos={repos} onChanged={loadRepos} canWrite={!isViewer} />

            {error ? <div className="notice danger" role="alert">{error}</div> : null}

            {repos.length > 0 && (
                <>
                    <div className="flex flex-wrap items-center gap-2">
                        <div className="relative min-w-[14rem] flex-1">
                            <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" aria-hidden="true" />
                            <Input
                                aria-label="Search charts"
                                data-testid={T.search}
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                placeholder="Search charts by name, description or keyword"
                                className="bg-muted/30 pl-9"
                            />
                        </div>
                        <select aria-label="Repository" data-testid={T.repoFilter} className="select" value={repoFilter} onChange={(e) => setRepoFilter(e.target.value)}>
                            <option value="">All repositories</option>
                            {repos.map((r) => (
                                <option key={r.name} value={r.name}>{r.name}</option>
                            ))}
                        </select>
                    </div>

                    {errors.length > 0 && (
                        <div className="notice" role="status" data-testid={T.errors}>
                            Some repositories could not be read: {errors.map(([n, m]) => `${n} (${m})`).join("; ")}
                        </div>
                    )}

                    {loading && !result ? <Skeleton className="h-40 rounded-2xl bg-muted" /> : null}
                    {result && result.charts.length === 0 && !loading ? (
                        <div className="card"><div className="empty" data-testid={T.empty}>No charts match.</div></div>
                    ) : null}
                    {result && result.charts.length > 0 && (
                        <>
                            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                                {result.charts.map((c) => (
                                    <button
                                        key={`${c.repo}/${c.name}`}
                                        type="button"
                                        data-testid={T.card(c.repo, c.name)}
                                        onClick={() => setSelected(c)}
                                        className="card flex flex-col gap-1.5 p-4 text-left transition-colors hover:bg-muted/40"
                                    >
                                        <div className="flex items-start justify-between gap-2">
                                            <span className="text-[15px] font-semibold">{c.name}</span>
                                            <span className="mono shrink-0 text-xs text-ink-3">{c.version}</span>
                                        </div>
                                        <p className="line-clamp-2 text-[12.5px] text-ink-3">{c.description || "No description."}</p>
                                        <div className="mt-auto flex items-center gap-2 pt-1 text-[11.5px] text-ink-3">
                                            <span className="pill soft">{c.repo}</span>
                                            {c.appVersion ? <span>app {c.appVersion}</span> : null}
                                            {c.deprecated ? <span className="text-tone-amber">deprecated</span> : null}
                                        </div>
                                    </button>
                                ))}
                            </div>
                            <p className="text-xs text-ink-3">
                                Showing {result.charts.length} of {result.total} charts.
                            </p>
                        </>
                    )}
                </>
            )}

            <ChartDialog chart={selected} repo={selected ? repoByName[selected.repo] || { url: "" } : null} onClose={() => setSelected(null)} onInstalled={onInstalled} />
        </div>
    );
}
