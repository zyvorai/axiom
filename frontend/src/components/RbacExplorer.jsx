import { Fragment, useEffect, useId, useMemo, useState } from "react";
import { Search, ShieldAlert, ShieldCheck, Users, User, Bot } from "lucide-react";
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
    return e?.response?.data?.error || e?.message || "Request failed";
}

function Denied({ status }) {
    return (
        <div className="empty" data-testid={RBAC.denied} role="status">
            {status === 403
                ? "The RBAC viewer lists every subject's permissions, so it needs the operator or admin role and an account that is not limited to namespaces."
                : "Could not load RBAC data."}
        </div>
    );
}

function RuleList({ rules }) {
    if (!rules || rules.length === 0) return <div className="text-[12.5px] text-ink-3">No rules (the referenced role is empty or missing).</div>;
    return (
        <div className="max-h-[260px] overflow-auto" tabIndex={0} role="region" aria-label="Rules">
            <table className="tbl">
                <thead>
                    <tr>
                        <th>API groups</th>
                        <th>Resources</th>
                        <th>Verbs</th>
                    </tr>
                </thead>
                <tbody>
                    {rules.map((r, i) => (
                        <tr key={i}>
                            <td className="mono wrap">{(r.apiGroups || []).map((g) => g || "core").join(", ") || (r.nonResourceURLs || []).join(", ") || "—"}</td>
                            <td className="mono wrap">
                                {(r.resources || []).join(", ") || "—"}
                                {r.resourceNames && r.resourceNames.length > 0 && <small>names: {r.resourceNames.join(", ")}</small>}
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
    return (
        <div className="card p-0" data-testid={RBAC.grant(g.binding)}>
            <div className="panel-h">
                <div className="min-w-0">
                    <div className="mono truncate text-[13px]">{g.binding}</div>
                    <div className="text-[12px] text-ink-3">
                        grants <span className="mono">{g.roleRef}</span>
                        {g.scope === "namespace" ? <> in <span className="mono">{g.namespace}</span></> : " in every namespace"}
                        {g.via && <> · via implicit group <span className="mono">{g.via}</span></>}
                    </div>
                </div>
                {g.dangling && (
                    <span className="pill" style={{ "--c": "var(--orange)" }}>
                        <i aria-hidden="true" />
                        role missing
                    </span>
                )}
            </div>
            <RuleList rules={g.rules} />
        </div>
    );
}

function SubjectDetail({ subject, onClose }) {
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
                    Close
                </button>
            </div>
            {state.loading && <Skeleton className="h-32 rounded-2xl bg-muted" />}
            {state.error && <div className="empty">{state.error}</div>}
            {state.data && (
                <>
                    {state.data.subject.clusterAdmin && (
                        <div className="notice danger" role="status">
                            <ShieldAlert size={16} aria-hidden="true" /> This subject has cluster-admin-equivalent permissions (all verbs on all resources).
                        </div>
                    )}
                    <h4 className="text-[13px] font-[500]">Cluster-wide ({state.data.cluster.length})</h4>
                    {state.data.cluster.length === 0 && <div className="text-[12.5px] text-ink-3">No ClusterRoleBindings reach this subject.</div>}
                    {state.data.cluster.map((g) => (
                        <GrantCard key={g.binding} g={g} />
                    ))}
                    <h4 className="text-[13px] font-[500]">Namespaced ({state.data.namespaced.length})</h4>
                    {state.data.namespaced.length === 0 && <div className="text-[12.5px] text-ink-3">No RoleBindings reach this subject.</div>}
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
                        aria-label="Search subjects"
                        data-testid={RBAC.subjectSearch}
                        placeholder="Search users, groups and service accounts"
                        value={q}
                        onChange={(e) => setQ(e.target.value)}
                        className="bg-muted/30 pl-[34px]"
                    />
                </div>
                <label htmlFor={kindId} className="sr-only">
                    Subject kind
                </label>
                <select
                    id={kindId}
                    data-testid={RBAC.subjectKind}
                    value={kind}
                    onChange={(e) => setKind(e.target.value)}
                    className="h-10 rounded-xl border border-input bg-card px-3 text-sm"
                >
                    <option value="all">All kinds</option>
                    <option value="User">Users</option>
                    <option value="Group">Groups</option>
                    <option value="ServiceAccount">Service accounts</option>
                </select>
            </div>
            <div className="grid g2">
                <div className="card p-0">
                    {rows.length === 0 ? (
                        <div className="empty">No subjects match.</div>
                    ) : (
                        <div className="max-h-[560px] overflow-auto" tabIndex={0} role="region" aria-label="Subjects">
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>Subject</th>
                                        <th>Bindings</th>
                                        <th>Reach</th>
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
                                                        aria-label={`View permissions of ${s.kind} ${s.name}`}
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
                                                            cluster-wide
                                                        </span>
                                                    ) : s.bindings > 0 ? (
                                                        <span className="pill" style={{ "--c": "var(--blue)" }}>
                                                            <i aria-hidden="true" />
                                                            namespaced
                                                        </span>
                                                    ) : (
                                                        <span className="text-[12.5px] text-ink-3">no bindings</span>
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
                        <div className="empty">Select a subject to see its effective permissions.</div>
                    )}
                </div>
            </div>
        </div>
    );
}

const VERBS = ["get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "*"];

export function RbacWhoCan() {
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
                        <Label htmlFor={`${id}-verb`}>Verb</Label>
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
                        <Label htmlFor={`${id}-res`}>Resource</Label>
                        <Input
                            id={`${id}-res`}
                            data-testid={RBAC.whoResource}
                            className="mt-1.5"
                            value={resource}
                            onChange={(e) => setResource(e.target.value)}
                            placeholder="pods, pods/exec, secrets…"
                        />
                    </div>
                    <div>
                        <Label htmlFor={`${id}-ns`}>Namespace (optional)</Label>
                        <Input
                            id={`${id}-ns`}
                            data-testid={RBAC.whoNamespace}
                            className="mt-1.5"
                            value={namespace}
                            onChange={(e) => setNamespace(e.target.value)}
                            placeholder="all namespaces"
                        />
                    </div>
                </div>
                <div className="mt-3 flex flex-wrap items-end gap-3">
                    <div className="min-w-[220px] flex-1">
                        <Label htmlFor={`${id}-name`}>Object name (optional)</Label>
                        <Input
                            id={`${id}-name`}
                            data-testid={RBAC.whoName}
                            className="mt-1.5"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder="matches rules restricted to named objects"
                        />
                    </div>
                    <button type="submit" className="btn btn-primary" data-testid={RBAC.whoRun} disabled={busy || !resource.trim()}>
                        <ShieldCheck size={14} /> {busy ? "Checking…" : "Who can?"}
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
                            Who can <span className="mono">{result.query.verb}</span> <span className="mono">{result.query.resource}</span>
                            {result.query.namespace ? <> in <span className="mono">{result.query.namespace}</span></> : " (any namespace)"}
                        </div>
                        <span className="r text-[12.5px] text-ink-3">{result.allowed.length} grant(s)</span>
                    </div>
                    {result.allowed.length === 0 ? (
                        <div className="empty">No binding grants this.</div>
                    ) : (
                        <div className="max-h-[480px] overflow-auto" tabIndex={0} role="region" aria-label="Who can results">
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>Subject</th>
                                        <th>Scope</th>
                                        <th>Via</th>
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
                                                <td>{a.scope === "cluster" ? "cluster-wide" : `namespace ${a.namespace}`}</td>
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
                    {result.truncated && <div className="px-4 pb-3 text-[12px] text-ink-3">Showing the first {result.allowed.length} results.</div>}
                </div>
            )}
        </div>
    );
}
