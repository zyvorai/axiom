import { Fragment, useEffect, useId, useMemo, useState } from "react";
import { Search, ShieldAlert, ShieldCheck, Users, User, Bot } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RBAC } from "@/constants/testIds";

// Read-only RBAC viewer (Ops -> Access). Two views over GET /rbac/subjects,
// /rbac/subjects/{kind}/{name} and /rbac/who-can. There is deliberately no
// write action here: Axiom does not write ClusterRoles (docs/GAPS.md).

const KIND_ICON = { User, Group: Users, ServiceAccount: Bot };

function errorText(e) {
    return e?.response?.data?.error || e?.message || i18n.t("errors:rbac.requestFailed");
}

function Denied({ status }) {
    const { t } = useTranslation();
    return (
        <div className="empty" data-testid={RBAC.denied} role="status">
            {status === 403
                ? t("pages:rbac.denied")
                : t("pages:rbac.loadFailed")}
        </div>
    );
}

function RuleList({ rules }) {
    const { t } = useTranslation();
    if (!rules || rules.length === 0) return <div className="text-[12.5px] text-ink-3">{t("pages:rbac.noRules")}</div>;
    return (
        <div className="max-h-[260px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:rbac.rules")}>
            <table className="tbl">
                <thead>
                    <tr>
                        <th>{t("pages:rbac.columns.apiGroups")}</th>
                        <th>{t("pages:rbac.columns.resources")}</th>
                        <th>{t("pages:rbac.columns.verbs")}</th>
                    </tr>
                </thead>
                <tbody>
                    {rules.map((r, i) => (
                        <tr key={i}>
                            <td className="mono wrap">{(r.apiGroups || []).map((g) => g || "core").join(", ") || (r.nonResourceURLs || []).join(", ") || "—"}</td>
                            <td className="mono wrap">
                                {(r.resources || []).join(", ") || "—"}
                                {r.resourceNames && r.resourceNames.length > 0 && <small>{t("pages:rbac.names", { names: r.resourceNames.join(", ") })}</small>}
                            </td>
                            <td className="mono wrap">{(r.verbs || []).join(", ")}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

function GrantCard({ g }) {
    const { t } = useTranslation();
    return (
        <div className="card p-0" data-testid={RBAC.grant(g.binding)}>
            <div className="panel-h">
                <div className="min-w-0">
                    <div className="mono truncate text-[13px]">{g.binding}</div>
                    <div className="text-[12px] text-ink-3">
                        {g.scope === "namespace" ? (
                            <Trans i18nKey="pages:rbac.grantsIn" values={{ role: g.roleRef, namespace: g.namespace }} components={{ code: <span className="mono" /> }} />
                        ) : (
                            <Trans i18nKey="pages:rbac.grantsEverywhere" values={{ role: g.roleRef }} components={{ code: <span className="mono" /> }} />
                        )}
                        {g.via && (
                            <>
                                {" "}
                                <Trans i18nKey="pages:rbac.viaGroup" values={{ group: g.via }} components={{ code: <span className="mono" /> }} />
                            </>
                        )}
                    </div>
                </div>
                {g.dangling && (
                    <span className="pill" style={{ "--c": "var(--orange)" }}>
                        <i aria-hidden="true" />
                        {t("pages:rbac.roleMissing")}
                    </span>
                )}
            </div>
            <RuleList rules={g.rules} />
        </div>
    );
}

function SubjectDetail({ subject, onClose }) {
    const { t } = useTranslation();
    const [state, setState] = useState({ loading: true });
    useEffect(() => {
        let alive = true;
        setState({ loading: true });
        const q = subject.namespace ? `?namespace=${encodeURIComponent(subject.namespace)}` : "";
        api
            .get(`/rbac/subjects/${encodeURIComponent(subject.kind)}/${encodeURIComponent(subject.name)}${q}`)
            .then((r) => alive && setState({ data: r.data }))
            .catch((e) => alive && setState({ error: errorText(e) }));
        return () => {
            alive = false;
        };
    }, [subject.kind, subject.name, subject.namespace]);

    return (
        <div className="stack" data-testid={RBAC.subjectDetail}>
            <div className="flex items-center gap-3">
                <h3 className="text-[15px] font-[500]">
                    {subject.kind} <span className="mono">{subject.namespace ? `${subject.namespace}/` : ""}{subject.name}</span>
                </h3>
                <button type="button" className="btn btn-line btn-sm ml-auto" onClick={onClose}>
                    {t("common:actions.close")}
                </button>
            </div>
            {state.loading && <Skeleton className="h-32 rounded-2xl bg-muted" />}
            {state.error && <div className="empty">{state.error}</div>}
            {state.data && (
                <>
                    {state.data.subject.clusterAdmin && (
                        <div className="notice danger" role="status">
                            <ShieldAlert size={16} aria-hidden="true" /> {t("pages:rbac.clusterAdminWarning")}
                        </div>
                    )}
                    <h4 className="text-[13px] font-[500]">{t("pages:rbac.clusterWideCount", { n: state.data.cluster.length })}</h4>
                    {state.data.cluster.length === 0 && <div className="text-[12.5px] text-ink-3">{t("pages:rbac.noClusterBindings")}</div>}
                    {state.data.cluster.map((g) => (
                        <GrantCard key={g.binding} g={g} />
                    ))}
                    <h4 className="text-[13px] font-[500]">{t("pages:rbac.namespacedCount", { n: state.data.namespaced.length })}</h4>
                    {state.data.namespaced.length === 0 && <div className="text-[12.5px] text-ink-3">{t("pages:rbac.noRoleBindings")}</div>}
                    {state.data.namespaced.map((g) => (
                        <GrantCard key={g.binding} g={g} />
                    ))}
                    {(state.data.notes || []).map((n) => (
                        <p key={n} className="text-[12px] text-ink-3">
                            {n}
                        </p>
                    ))}
                </>
            )}
        </div>
    );
}

export function RbacBySubject() {
    const { t } = useTranslation();
    const [subjects, setSubjects] = useState(null);
    const [error, setError] = useState(null);
    const [q, setQ] = useState("");
    const [kind, setKind] = useState("all");
    const [selected, setSelected] = useState(null);
    const kindId = useId();

    useEffect(() => {
        let alive = true;
        api
            .get("/rbac/subjects")
            .then((r) => alive && setSubjects(r.data || []))
            .catch((e) => alive && setError(e?.response?.status || 0));
        return () => {
            alive = false;
        };
    }, []);

    const rows = useMemo(() => {
        if (!subjects) return [];
        const needle = q.trim().toLowerCase();
        return subjects.filter(
            (s) => (kind === "all" || s.kind === kind) && (!needle || `${s.namespace || ""}/${s.name}`.toLowerCase().includes(needle))
        );
    }, [subjects, q, kind]);

    if (error !== null) return <Denied status={error} />;
    if (!subjects) return <Skeleton className="h-48 rounded-2xl bg-muted" />;

    return (
        <div className="stack" data-testid={RBAC.subjectsRoot}>
            <div className="flex flex-wrap items-center gap-2.5">
                <div className="relative min-w-[220px] flex-1">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" aria-hidden="true" />
                    <Input
                        aria-label={t("pages:rbac.searchSubjects")}
                        data-testid={RBAC.subjectSearch}
                        placeholder={t("pages:rbac.searchPlaceholder")}
                        value={q}
                        onChange={(e) => setQ(e.target.value)}
                        className="bg-muted/30 pl-[34px]"
                    />
                </div>
                <label htmlFor={kindId} className="sr-only">
                    {t("pages:rbac.subjectKind")}
                </label>
                <select
                    id={kindId}
                    data-testid={RBAC.subjectKind}
                    value={kind}
                    onChange={(e) => setKind(e.target.value)}
                    className="h-10 rounded-xl border border-input bg-card px-3 text-sm"
                >
                    <option value="all">{t("pages:rbac.kinds.all")}</option>
                    <option value="User">{t("pages:rbac.kinds.users")}</option>
                    <option value="Group">{t("pages:rbac.kinds.groups")}</option>
                    <option value="ServiceAccount">{t("pages:rbac.kinds.serviceAccounts")}</option>
                </select>
            </div>
            <div className="grid g2">
                <div className="card p-0">
                    {rows.length === 0 ? (
                        <div className="empty">{t("pages:rbac.noSubjects")}</div>
                    ) : (
                        <div className="max-h-[560px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:rbac.subjects")}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>{t("pages:rbac.columns.subject")}</th>
                                        <th>{t("pages:rbac.columns.bindings")}</th>
                                        <th>{t("pages:rbac.columns.reach")}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {rows.map((s) => {
                                        const Icon = KIND_ICON[s.kind] || User;
                                        const key = `${s.kind}/${s.namespace || ""}/${s.name}`;
                                        const active = selected && `${selected.kind}/${selected.namespace || ""}/${selected.name}` === key;
                                        return (
                                            <tr key={key} data-testid={RBAC.subjectRow(s.kind, s.name)} aria-selected={active || undefined}>
                                                <td className="name">
                                                    <button
                                                        type="button"
                                                        className="flex items-center gap-2 text-left"
                                                        onClick={() => setSelected(s)}
                                                        aria-label={t("pages:rbac.viewPermissions", { kind: s.kind, name: s.name })}
                                                    >
                                                        <Icon size={14} className="text-ink-3" aria-hidden="true" />
                                                        <span>{s.name}</span>
                                                    </button>
                                                    <small>{s.kind}{s.namespace ? ` · ${s.namespace}` : ""}</small>
                                                </td>
                                                <td className="num">{s.bindings}</td>
                                                <td>
                                                    {s.clusterAdmin ? (
                                                        <span className="pill" style={{ "--c": "var(--red)" }}>
                                                            <i aria-hidden="true" />
                                                            cluster-admin
                                                        </span>
                                                    ) : s.clusterWide ? (
                                                        <span className="pill" style={{ "--c": "var(--orange)" }}>
                                                            <i aria-hidden="true" />
                                                            {t("pages:rbac.clusterWide")}
                                                        </span>
                                                    ) : s.bindings > 0 ? (
                                                        <span className="pill" style={{ "--c": "var(--blue)" }}>
                                                            <i aria-hidden="true" />
                                                            {t("pages:rbac.namespaced")}
                                                        </span>
                                                    ) : (
                                                        <span className="text-[12.5px] text-ink-3">{t("pages:rbac.noBindings")}</span>
                                                    )}
                                                </td>
                                            </tr>
                                        );
                                    })}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
                <div className="card">
                    {selected ? (
                        <SubjectDetail subject={selected} onClose={() => setSelected(null)} />
                    ) : (
                        <div className="empty">{t("pages:rbac.selectSubject")}</div>
                    )}
                </div>
            </div>
        </div>
    );
}

const VERBS = ["get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "*"];

export function RbacWhoCan() {
    const { t } = useTranslation();
    const id = useId();
    const [verb, setVerb] = useState("get");
    const [resource, setResource] = useState("secrets");
    const [namespace, setNamespace] = useState("");
    const [name, setName] = useState("");
    const [busy, setBusy] = useState(false);
    const [result, setResult] = useState(null);
    const [error, setError] = useState(null);
    const [denied, setDenied] = useState(null);

    const run = async (e) => {
        e?.preventDefault();
        setBusy(true);
        setError(null);
        try {
            const params = { verb, resource: resource.trim() };
            if (namespace.trim()) params.namespace = namespace.trim();
            if (name.trim()) params.name = name.trim();
            const r = await api.get("/rbac/who-can", { params });
            setResult(r.data);
        } catch (err) {
            if (err?.response?.status === 403) setDenied(403);
            else setError(errorText(err));
            setResult(null);
        } finally {
            setBusy(false);
        }
    };

    if (denied !== null) return <Denied status={denied} />;

    return (
        <div className="stack" data-testid={RBAC.whoRoot}>
            <form className="card" onSubmit={run}>
                <div className="grid g3">
                    <div>
                        <Label htmlFor={`${id}-verb`}>{t("pages:rbac.who.verb")}</Label>
                        <select
                            id={`${id}-verb`}
                            data-testid={RBAC.whoVerb}
                            value={verb}
                            onChange={(e) => setVerb(e.target.value)}
                            className="mt-1.5 h-10 w-full rounded-xl border border-input bg-card px-3 text-sm"
                        >
                            {VERBS.map((v) => (
                                <option key={v} value={v}>
                                    {v}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div>
                        <Label htmlFor={`${id}-res`}>{t("pages:rbac.who.resource")}</Label>
                        <Input
                            id={`${id}-res`}
                            data-testid={RBAC.whoResource}
                            className="mt-1.5"
                            value={resource}
                            onChange={(e) => setResource(e.target.value)}
                            placeholder={t("pages:rbac.who.resourcePlaceholder")}
                        />
                    </div>
                    <div>
                        <Label htmlFor={`${id}-ns`}>{t("pages:rbac.who.namespace")}</Label>
                        <Input
                            id={`${id}-ns`}
                            data-testid={RBAC.whoNamespace}
                            className="mt-1.5"
                            value={namespace}
                            onChange={(e) => setNamespace(e.target.value)}
                            placeholder={t("pages:rbac.who.namespacePlaceholder")}
                        />
                    </div>
                </div>
                <div className="mt-3 flex flex-wrap items-end gap-3">
                    <div className="min-w-[220px] flex-1">
                        <Label htmlFor={`${id}-name`}>{t("pages:rbac.who.name")}</Label>
                        <Input
                            id={`${id}-name`}
                            data-testid={RBAC.whoName}
                            className="mt-1.5"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder={t("pages:rbac.who.namePlaceholder")}
                        />
                    </div>
                    <button type="submit" className="btn btn-primary" data-testid={RBAC.whoRun} disabled={busy || !resource.trim()}>
                        <ShieldCheck size={14} /> {busy ? t("pages:rbac.who.checking") : t("pages:rbac.who.run")}
                    </button>
                </div>
            </form>
            {error && (
                <div className="notice danger" role="alert">
                    {error}
                </div>
            )}
            {result && (
                <div className="card p-0">
                    <div className="panel-h">
                        <div className="text-[14px] font-[500]">
                            {result.query.namespace ? (
                                <Trans
                                    i18nKey="pages:rbac.who.titleIn"
                                    values={{ verb: result.query.verb, resource: result.query.resource, namespace: result.query.namespace }}
                                    components={{ code: <span className="mono" /> }}
                                />
                            ) : (
                                <Trans
                                    i18nKey="pages:rbac.who.titleAny"
                                    values={{ verb: result.query.verb, resource: result.query.resource }}
                                    components={{ code: <span className="mono" /> }}
                                />
                            )}
                        </div>
                        <span className="r text-[12.5px] text-ink-3">{t("pages:rbac.who.grants", { n: result.allowed.length })}</span>
                    </div>
                    {result.allowed.length === 0 ? (
                        <div className="empty">{t("pages:rbac.who.none")}</div>
                    ) : (
                        <div className="max-h-[480px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:rbac.who.results")}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>{t("pages:rbac.columns.subject")}</th>
                                        <th>{t("pages:rbac.columns.scope")}</th>
                                        <th>{t("pages:rbac.columns.via")}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {result.allowed.map((a, i) => (
                                        <Fragment key={i}>
                                            <tr data-testid={RBAC.whoRow(a.subject.kind, a.subject.name, a.scope)}>
                                                <td className="name">
                                                    {a.subject.name}
                                                    <small>{a.subject.kind}{a.subject.namespace ? ` · ${a.subject.namespace}` : ""}</small>
                                                </td>
                                                <td>{a.scope === "cluster" ? t("pages:rbac.clusterWide") : t("pages:rbac.who.inNamespace", { namespace: a.namespace })}</td>
                                                <td className="mono wrap">
                                                    {a.binding}
                                                    <small className="block">{a.roleRef}</small>
                                                </td>
                                            </tr>
                                        </Fragment>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                    {result.truncated && <div className="px-4 pb-3 text-[12px] text-ink-3">{t("pages:rbac.who.truncated", { n: result.allowed.length })}</div>}
                </div>
            )}
        </div>
    );
}
