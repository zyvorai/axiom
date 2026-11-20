import { useCallback, useEffect, useMemo, useState } from "react";
import { Download, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useTranslation, Trans } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useConfirm } from "@/components/ConfirmDialog";
import MacTerm from "@/components/MacTerm";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { HELM_CATALOG as T, OCI, REGCRED } from "@/constants/testIds";
import RegistryLogins, { RegistryLoginsButton } from "@/components/RegistryLogins";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:helmCatalog.generic");

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
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [name, setName] = useState("");
    const [url, setUrl] = useState("");
    const [kind, setKind] = useState("index");
    const [charts, setCharts] = useState("");
    const [busy, setBusy] = useState(false);
    const [logins, setLogins] = useState([]);
    const [login, setLogin] = useState("");
    const [loginsOpen, setLoginsOpen] = useState(false);

    // Stored registry logins (admin-only; anyone else just sees none).
    const loadLogins = useCallback(() => {
        if (!canWrite) return;
        api.get("/helm/registry-credentials").then((r) => setLogins(r.data || [])).catch(() => setLogins([]));
    }, [canWrite]);
    useEffect(() => {
        loadLogins();
    }, [loadLogins]);

    const add = async (n, u) => {
        setBusy(true);
        try {
            const body = { name: n, url: u };
            if (kind === "oci") {
                // A registry has no index: list the charts to browse (most hosted registries refuse the catalog API).
                body.charts = charts.split(/[\s,]+/).map((c) => c.trim()).filter(Boolean);
                if (login) body.credentialsSecret = login;
            }
            await api.post("/helm/repos", body);
            toast.success(t("toasts:helmCatalog.repoAdded", { name: n }));
            setName("");
            setUrl("");
            setCharts("");
            setLogin("");
            onChanged();
        } catch (e) {
            toast.error(t("toasts:helmCatalog.repoAddFailed"), { description: errText(e) });
        } finally {
            setBusy(false);
        }
    };

    const remove = async (r) => {
        const ok = await confirm({
            title: t("dialogs:helmCatalog.removeRepo.title", { name: r.name }),
            description: t("dialogs:helmCatalog.removeRepo.description"),
            confirmLabel: t("common:actions.remove"),
        });
        if (!ok) return;
        try {
            await api.delete(`/helm/repos/${encodeURIComponent(r.name)}`);
            toast.success(t("toasts:helmCatalog.repoRemoved", { name: r.name }));
            onChanged();
        } catch (e) {
            toast.error(t("toasts:helmCatalog.removeFailed"), { description: errText(e) });
        }
    };

    const known = new Set(repos.map((r) => r.name));
    return (
        <div className="card space-y-3 p-4">
            <div className="text-sm font-medium">{t("pages:helmCatalog.repositories")}</div>
            {repos.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                    {t("pages:helmCatalog.noRepos")}
                </p>
            ) : (
                <ul className="divide-y divide-border">
                    {repos.map((r) => (
                        <li key={r.name} className="flex items-center justify-between gap-3 py-2" data-testid={T.repoRow(r.name)}>
                            <div className="min-w-0">
                                <div className="text-sm font-medium">
                                    {r.name}
                                    {r.kind === "oci" ? <span className="pill soft ml-2 text-[10px]">OCI</span> : null}
                                    {r.authenticated ? <span className="pill soft ml-1 text-[10px]" data-testid={REGCRED.locked(r.name)} title={t("pages:helmCatalog.privateTitle", { name: r.credentialsSecret })}>{t("pages:helmCatalog.private")}</span> : null}
                                </div>
                                <div className="mono truncate text-[11.5px] text-ink-3" title={r.url}>{r.url}</div>
                                {r.error ? <div className="text-xs text-tone-red">{r.error}</div> : null}
                            </div>
                            <div className="flex shrink-0 items-center gap-2 text-xs text-ink-3">
                                {r.chartCount ? <span>{t("pages:helmCatalog.chartCount", { n: r.chartCount })}</span> : null}
                                {canWrite && (
                                    <button type="button" className="btn btn-danger btn-sm" data-testid={T.repoRemove(r.name)} aria-label={t("pages:helmCatalog.removeRepoAria", { name: r.name })} onClick={() => remove(r)}>
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
                    <div className="flex items-center gap-4 text-sm" role="radiogroup" aria-label={t("pages:helmCatalog.oci.kindLabel")} data-testid={OCI.kind}>
                        <label className="flex cursor-pointer items-center gap-2">
                            <input type="radio" name="helm-repo-kind" checked={kind === "index"} onChange={() => setKind("index")} className="accent-primary" />
                            {t("pages:helmCatalog.oci.index")}
                        </label>
                        <label className="flex cursor-pointer items-center gap-2">
                            <input type="radio" name="helm-repo-kind" data-testid={OCI.kindOci} checked={kind === "oci"} onChange={() => setKind("oci")} className="accent-primary" />
                            {t("pages:helmCatalog.oci.registry")}
                        </label>
                    </div>
                    <form
                        className="grid gap-2 sm:grid-cols-[10rem_1fr_auto]"
                        onSubmit={(e) => {
                            e.preventDefault();
                            if (name && url) add(name.trim(), url.trim());
                        }}
                    >
                        <div>
                            <Label htmlFor="helm-repo-name" className="sr-only">{t("pages:helmCatalog.repoName")}</Label>
                            <Input id="helm-repo-name" data-testid={T.repoName} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("pages:helmCatalog.repoNamePlaceholder")} className="bg-muted/30" />
                        </div>
                        <div>
                            <Label htmlFor="helm-repo-url" className="sr-only">{t("pages:helmCatalog.repoUrl")}</Label>
                            <Input id="helm-repo-url" data-testid={T.repoUrl} value={url} onChange={(e) => setUrl(e.target.value)} placeholder={kind === "oci" ? "oci://ghcr.io/org/charts" : "https://charts.example.com"} className="bg-muted/30" />
                        </div>
                        {kind === "oci" && (
                            <div className="flex flex-wrap items-center gap-2 sm:col-span-3">
                                <Label htmlFor="helm-repo-login" className="text-xs text-muted-foreground">{t("pages:helmCatalog.oci.login")}</Label>
                                <select id="helm-repo-login" data-testid={REGCRED.select} className="select" value={login} onChange={(e) => setLogin(e.target.value)}>
                                    <option value="">{t("pages:helmCatalog.oci.noLogin")}</option>
                                    {logins.map((l) => (
                                        <option key={l.name} value={l.name}>{t("pages:helmCatalog.oci.loginOption", { name: l.name, registry: l.registry })}</option>
                                    ))}
                                </select>
                                <RegistryLoginsButton onClick={() => setLoginsOpen(true)} />
                            </div>
                        )}
                        {kind === "oci" && (
                            <div className="sm:col-span-3">
                                <Label htmlFor="helm-repo-oci-charts" className="sr-only">{t("pages:helmCatalog.oci.chartNames")}</Label>
                                <Input id="helm-repo-oci-charts" data-testid={OCI.charts} value={charts} onChange={(e) => setCharts(e.target.value)} placeholder={t("pages:helmCatalog.oci.chartNamesPlaceholder")} className="bg-muted/30" />
                            </div>
                        )}
                        <button type="submit" className="btn btn-fill btn-sm" data-testid={T.repoAdd} disabled={busy || !name || !url || (kind === "oci" && !charts.trim())}>
                            <Plus size={14} /> {busy ? t("pages:helmCatalog.adding") : t("common:actions.add")}
                        </button>
                    </form>
                    <div className={`flex flex-wrap items-center gap-1.5 text-xs text-ink-3 ${kind === "oci" ? "hidden" : ""}`}>
                        <span>{t("pages:helmCatalog.suggested")}</span>
                        {SUGGESTED.filter((s) => !known.has(s.name)).map((s) => (
                            <button key={s.name} type="button" className="pill soft" data-testid={T.repoSuggest(s.name)} disabled={busy} onClick={() => add(s.name, s.url)}>
                                + {s.name}
                            </button>
                        ))}
                    </div>
                </>
            )}
            <RegistryLogins open={loginsOpen} onOpenChange={setLoginsOpen} onChanged={() => { loadLogins(); onChanged(); }} />
        </div>
    );
}

function ChartDialog({ chart, repo, onClose, onInstalled }) {
    const { t } = useTranslation();
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
            await api.post(
                "/helm/install",
                chart.kind === "oci"
                    ? { namespace, releaseName: release, ociRef: chart.ref, chartVersion: version, values }
                    : { namespace, releaseName: release, repoUrl: repo.url, chartName: chart.name, chartVersion: version, values }
            );
            toast.success(t("toasts:helmCatalog.installStarted", { name: release }), { description: t("toasts:helmCatalog.installStartedDescription") });
            onInstalled();
            onClose();
        } catch (e) {
            toast.error(t("toasts:helmCatalog.installFailed"), { description: errText(e) });
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
                        {chart.description || t("pages:helmCatalog.noDescription")} · <Trans i18nKey="dialogs:helmCatalog.chart.from" values={{ repo: chart.repo }} components={{ code: <span className="mono" /> }} />
                        {chart.kind === "oci" && chart.ref ? (
                            <span className="mono mt-1 block text-[11.5px]" data-testid={OCI.ref}>{chart.ref}</span>
                        ) : null}
                    </DialogDescription>
                </DialogHeader>

                <div tabIndex={0} role="region" aria-label={t("dialogs:helmCatalog.chart.optionsRegion")} className="max-h-[65vh] space-y-4 overflow-y-auto pr-1">
                <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5">
                        <Label htmlFor="helm-chart-version" className="text-xs text-muted-foreground">{t("common:fields.version")}</Label>
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
                                    {v.appVersion ? t("dialogs:helmCatalog.chart.appSuffix", { version: v.appVersion }) : ""}
                                    {v.deprecated ? t("dialogs:helmCatalog.chart.deprecatedSuffix") : ""}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="helm-chart-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                        <Input id="helm-chart-namespace" data-testid={T.namespace} value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5 sm:col-span-2">
                        <Label htmlFor="helm-chart-release" className="text-xs text-muted-foreground">{t("dialogs:helmCatalog.chart.releaseName")}</Label>
                        <Input id="helm-chart-release" data-testid={T.release} value={release} onChange={(e) => setRelease(e.target.value)} className="bg-muted/30" />
                    </div>
                </div>

                <div className="space-y-2">
                    <div className="flex items-center justify-between">
                        <span className="text-xs text-muted-foreground">{t("dialogs:helmCatalog.chart.values")}</span>
                        <button type="button" className="btn btn-line btn-sm" data-testid={T.showValues} onClick={loadValues} disabled={loadingVals}>
                            <Download size={13} /> {loadingVals ? t("common:states.loading") : vals ? t("dialogs:helmCatalog.chart.reloadDefaults") : t("dialogs:helmCatalog.chart.loadDefaults")}
                        </button>
                    </div>
                    {valsError ? <p className="text-xs text-tone-red" role="alert">{valsError}</p> : null}
                    <MacTerm title="values.yaml" bodyClassName="!p-0">
                        <Textarea
                            aria-label={t("dialogs:helmCatalog.chart.valuesYaml")}
                            data-testid={T.valuesEditor}
                            value={values}
                            onChange={(e) => setValues(e.target.value)}
                            placeholder={t("dialogs:helmCatalog.chart.valuesPlaceholder")}
                            className="macos-term-body mono min-h-[160px] resize-y rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0"
                            spellCheck={false}
                        />
                    </MacTerm>
                    {vals?.readme ? (
                        <div>
                            <button type="button" className="text-xs text-tone-link underline" onClick={() => setShowReadme((v) => !v)}>
                                {showReadme ? t("dialogs:helmCatalog.chart.hideReadme") : t("dialogs:helmCatalog.chart.showReadme")}
                            </button>
                            {showReadme ? (
                                <pre tabIndex={0} role="region" aria-label={t("dialogs:helmCatalog.chart.readme")} className="mono mt-2 max-h-64 overflow-auto rounded-xl bg-muted p-3 text-xs" data-testid={T.readme}>{vals.readme}</pre>
                            ) : null}
                        </div>
                    ) : null}
                    <p className="text-[11px] text-muted-foreground">
                        {t("dialogs:helmCatalog.chart.permissionsNote")}
                    </p>
                </div>

                </div>

                <DialogFooter>
                    <button type="button" className="btn btn-fill" data-testid={T.install} onClick={install} disabled={busy || !release || !namespace}>
                        {busy ? t("dialogs:helmCatalog.chart.starting") : t("dialogs:helmCatalog.chart.install", { version })}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

/** Browse and install charts from administrator-approved Helm repositories. */
export default function HelmCatalog({ onInstalled }) {
    const { t } = useTranslation();
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
        const timer = setTimeout(search, 250);
        return () => clearTimeout(timer);
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
                                aria-label={t("pages:helmCatalog.searchAria")}
                                data-testid={T.search}
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                placeholder={t("pages:helmCatalog.searchPlaceholder")}
                                className="bg-muted/30 pl-9"
                            />
                        </div>
                        <select aria-label={t("pages:helmCatalog.repository")} data-testid={T.repoFilter} className="select" value={repoFilter} onChange={(e) => setRepoFilter(e.target.value)}>
                            <option value="">{t("pages:helmCatalog.allRepositories")}</option>
                            {repos.map((r) => (
                                <option key={r.name} value={r.name}>{r.name}</option>
                            ))}
                        </select>
                    </div>

                    {errors.length > 0 && (
                        <div className="notice" role="status" data-testid={T.errors}>
                            {t("pages:helmCatalog.repoErrors", { list: errors.map(([n, m]) => `${n} (${m})`).join("; ") })}
                        </div>
                    )}

                    {loading && !result ? <Skeleton className="h-40 rounded-2xl bg-muted" /> : null}
                    {result && result.charts.length === 0 && !loading ? (
                        <div className="card"><div className="empty" data-testid={T.empty}>{t("pages:helmCatalog.noMatch")}</div></div>
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
                                        <p className="line-clamp-2 text-[12.5px] text-ink-3">{c.description || t("pages:helmCatalog.noDescription")}</p>
                                        <div className="mt-auto flex items-center gap-2 pt-1 text-[11.5px] text-ink-3">
                                            <span className="pill soft">{c.repo}</span>
                                            {c.kind === "oci" ? <span className="pill soft" data-testid={OCI.badge(c.repo, c.name)}>OCI</span> : null}
                                            {c.appVersion ? <span>{t("pages:helmCatalog.appVersion", { version: c.appVersion })}</span> : null}
                                            {c.deprecated ? <span className="text-tone-amber">{t("pages:helmCatalog.deprecated")}</span> : null}
                                        </div>
                                    </button>
                                ))}
                            </div>
                            <p className="text-xs text-ink-3">
                                {t("pages:helmCatalog.showing", { shown: result.charts.length, total: result.total })}
                            </p>
                        </>
                    )}
                </>
            )}

            <ChartDialog chart={selected} repo={selected ? repoByName[selected.repo] || { url: "" } : null} onClose={() => setSelected(null)} onInstalled={onInstalled} />
        </div>
    );
}
