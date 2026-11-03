import { useEffect, useMemo, useState } from "react";
import { Eye, Plus } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import MacTerm from "@/components/MacTerm";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { GITOPS_CREATE as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";

const ENGINES = [
    { key: "argocd", label: "Argo CD application", hint: "An Application that syncs a Git path or a Helm chart." },
    { key: "flux-kustomize", label: "Flux · Git + Kustomize", hint: "A GitRepository and a Kustomization that applies a path from it." },
    { key: "flux-helm", label: "Flux · Helm release", hint: "A HelmRepository and a HelmRelease that installs a chart." },
];

const INITIAL = {
    engine: "argocd",
    name: "",
    namespace: "",
    repoUrl: "",
    path: "",
    revision: "",
    chart: "",
    chartVersion: "",
    sourceKind: "git", // argocd only: git path or helm chart
    destNamespace: "",
    secretRef: "",
    interval: "5m",
    values: "",
    automated: false,
    prune: false,
    selfHeal: false,
    createNamespace: true,
};

// Turns the form into the API's typed request. Only fields that apply to the
// chosen engine are sent, so the server's strict validation sees a clean body.
export function buildRequest(f) {
    const flux = f.engine !== "argocd";
    const helmSource = f.engine === "flux-helm" || (f.engine === "argocd" && f.sourceKind === "helm");
    const source = { repoUrl: f.repoUrl.trim() };
    if (helmSource) {
        source.chart = f.chart.trim();
        if (f.chartVersion.trim()) source.chartVersion = f.chartVersion.trim();
    } else {
        source.path = f.path.trim() || ".";
    }
    if (f.revision.trim() && !helmSource) source.revision = f.revision.trim();
    if (flux && f.secretRef.trim()) source.secretRef = f.secretRef.trim();
    const req = {
        engine: f.engine,
        name: f.name.trim(),
        source,
        destination: { namespace: f.destNamespace.trim() },
    };
    if (f.namespace.trim()) req.namespace = f.namespace.trim();
    if (flux) req.interval = f.interval.trim() || "5m";
    if (helmSource && f.values.trim()) req.values = f.values;
    if (f.engine === "argocd") {
        req.syncPolicy = { automated: f.automated, prune: f.automated && f.prune, selfHeal: f.automated && f.selfHeal, createNamespace: f.createNamespace };
    } else if (f.engine === "flux-kustomize") {
        req.syncPolicy = { prune: f.prune };
    } else {
        req.syncPolicy = { createNamespace: f.createNamespace };
    }
    return req;
}

function Field({ id, label, testId, value, onChange, placeholder, hint, ...rest }) {
    return (
        <div className="space-y-1.5">
            <Label htmlFor={id} className="text-xs text-muted-foreground">{label}</Label>
            <Input id={id} data-testid={testId} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="bg-muted/30" {...rest} />
            {hint ? <p className="text-[11px] text-muted-foreground">{hint}</p> : null}
        </div>
    );
}

function Check({ id, label, testId, checked, onChange, disabled }) {
    return (
        <label htmlFor={id} className="flex cursor-pointer items-center gap-2 text-sm">
            <input id={id} data-testid={testId} type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" />
            {label}
        </label>
    );
}

/** Create an Argo CD / Flux application from typed fields, after reviewing the exact YAML Axiom will create. */
export default function GitOpsCreateDialog({ open, onOpenChange, onCreated }) {
    const [f, setF] = useState(INITIAL);
    const [preview, setPreview] = useState(null); // {yaml, objects, warnings}
    const [previewFor, setPreviewFor] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState("");
    const set = (patch) => setF((cur) => ({ ...cur, ...patch }));

    useEffect(() => {
        if (open) {
            setF(INITIAL);
            setPreview(null);
            setPreviewFor("");
            setError("");
        }
    }, [open]);

    const req = useMemo(() => buildRequest(f), [f]);
    const reqKey = JSON.stringify(req);
    // The preview is only valid for the exact request it was generated from.
    const previewCurrent = preview && previewFor === reqKey;

    const flux = f.engine !== "argocd";
    const helmSource = f.engine === "flux-helm" || (f.engine === "argocd" && f.sourceKind === "helm");
    const defaultNs = flux ? "flux-system" : "argocd";
    const complete = f.name && f.repoUrl && f.destNamespace && (helmSource ? f.chart : true);

    const doPreview = async () => {
        setBusy("preview");
        setError("");
        try {
            const { data } = await api.post("/gitops/preview", req);
            setPreview(data);
            setPreviewFor(reqKey);
        } catch (e) {
            setPreview(null);
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    const doCreate = async () => {
        setBusy("create");
        setError("");
        try {
            const { data } = await api.post("/gitops", req);
            const names = (data.created || []).map((o) => `${o.kind}/${o.name}`).join(", ");
            toast.success(`Created ${f.name}`, { description: names });
            onCreated();
            onOpenChange(false);
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-2xl" data-testid={T.dialog}>
                <DialogHeader>
                    <DialogTitle>New GitOps application</DialogTitle>
                    <DialogDescription>
                        Axiom builds the objects from these fields — you review the exact YAML before anything is created.
                    </DialogDescription>
                </DialogHeader>

                <div tabIndex={0} role="region" aria-label="Application details" className="max-h-[65vh] space-y-4 overflow-y-auto pr-1">
                <div role="radiogroup" aria-label="Engine" className="grid gap-2 sm:grid-cols-3">
                    {ENGINES.map((e) => (
                        <button
                            key={e.key}
                            type="button"
                            role="radio"
                            aria-checked={f.engine === e.key}
                            data-testid={T.engine(e.key)}
                            onClick={() => set({ engine: e.key })}
                            className={`rounded-xl border p-3 text-left text-sm transition-colors ${f.engine === e.key ? "border-primary bg-muted/60" : "border-border hover:bg-muted/30"}`}
                        >
                            <div className="font-medium">{e.label}</div>
                            <div className="mt-0.5 text-[11.5px] text-muted-foreground">{e.hint}</div>
                        </button>
                    ))}
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                    <Field id="gitops-name" label="Name" testId={T.name} value={f.name} onChange={(v) => set({ name: v })} placeholder="my-app" />
                    <Field id="gitops-namespace" label={`Namespace of the ${flux ? "Flux" : "Argo CD"} objects`} testId={T.namespace} value={f.namespace} onChange={(v) => set({ namespace: v })} placeholder={defaultNs} />
                    <div className="sm:col-span-2">
                        <Field
                            id="gitops-repo-url"
                            label={helmSource ? "Helm repository URL" : "Git repository URL"}
                            testId={T.repoUrl}
                            value={f.repoUrl}
                            onChange={(v) => set({ repoUrl: v })}
                            placeholder={helmSource ? "https://charts.example.com" : "https://github.com/org/repo.git"}
                            hint="Credentials never go in the URL; reference a Secret instead."
                        />
                    </div>

                    {f.engine === "argocd" && (
                        <div className="flex items-center gap-4 sm:col-span-2" role="radiogroup" aria-label="Source type">
                            {[
                                { k: "git", l: "Git path" },
                                { k: "helm", l: "Helm chart" },
                            ].map((o) => (
                                <label key={o.k} className="flex cursor-pointer items-center gap-2 text-sm">
                                    <input type="radio" name="gitops-source-kind" checked={f.sourceKind === o.k} onChange={() => set({ sourceKind: o.k })} className="accent-primary" />
                                    {o.l}
                                </label>
                            ))}
                        </div>
                    )}

                    {helmSource ? (
                        <>
                            <Field id="gitops-chart" label="Chart" testId={T.chart} value={f.chart} onChange={(v) => set({ chart: v })} placeholder="nginx" />
                            <Field id="gitops-chart-version" label="Chart version" testId={T.chartVersion} value={f.chartVersion} onChange={(v) => set({ chartVersion: v })} placeholder={flux ? "15.x" : "15.1.0"} />
                        </>
                    ) : (
                        <>
                            <Field id="gitops-path" label="Path in the repository" testId={T.path} value={f.path} onChange={(v) => set({ path: v })} placeholder="deploy/overlays/prod" />
                            <Field id="gitops-revision" label="Branch, tag or commit" testId={T.revision} value={f.revision} onChange={(v) => set({ revision: v })} placeholder={flux ? "main" : "HEAD"} />
                        </>
                    )}

                    <Field id="gitops-dest-namespace" label="Deploy into namespace" testId={T.destNamespace} value={f.destNamespace} onChange={(v) => set({ destNamespace: v })} placeholder="production" />
                    {flux ? (
                        <>
                            <Field id="gitops-secret-ref" label="Credentials Secret (optional)" testId={T.secretRef} value={f.secretRef} onChange={(v) => set({ secretRef: v })} placeholder="repo-credentials" hint="Name of an existing Secret in the Flux namespace." />
                            <Field id="gitops-interval" label="Reconcile interval" testId={T.interval} value={f.interval} onChange={(v) => set({ interval: v })} placeholder="5m" />
                        </>
                    ) : null}
                </div>

                {helmSource && (
                    <div className="space-y-1.5">
                        <Label htmlFor="gitops-values" className="text-xs text-muted-foreground">Values YAML (optional)</Label>
                        <MacTerm title="values.yaml" bodyClassName="!p-0">
                            <Textarea id="gitops-values" data-testid={T.values} value={f.values} onChange={(e) => set({ values: e.target.value })} className="macos-term-body mono min-h-[90px] resize-y rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                )}

                <fieldset className="space-y-2 rounded-xl border border-border p-3">
                    <legend className="px-1 text-xs text-muted-foreground">Sync</legend>
                    {f.engine === "argocd" && (
                        <>
                            <Check id="gitops-automated" testId={T.automated} label="Sync automatically when the repository changes" checked={f.automated} onChange={(v) => set({ automated: v })} />
                            <div className="ml-6 space-y-2">
                                <Check id="gitops-prune" testId={T.prune} label="Delete resources removed from Git (prune)" checked={f.prune} disabled={!f.automated} onChange={(v) => set({ prune: v })} />
                                <Check id="gitops-selfheal" testId={T.selfHeal} label="Revert manual changes (self-heal)" checked={f.selfHeal} disabled={!f.automated} onChange={(v) => set({ selfHeal: v })} />
                            </div>
                        </>
                    )}
                    {f.engine === "flux-kustomize" && (
                        <Check id="gitops-prune" testId={T.prune} label="Delete resources removed from Git (prune)" checked={f.prune} onChange={(v) => set({ prune: v })} />
                    )}
                    {f.engine !== "flux-kustomize" && (
                        <Check id="gitops-create-namespace" testId={T.createNamespace} label="Create the target namespace if it does not exist" checked={f.createNamespace} onChange={(v) => set({ createNamespace: v })} />
                    )}
                </fieldset>

                {error ? (
                    <div className="notice danger" role="alert" data-testid={T.error}>{error}</div>
                ) : null}

                {preview && (
                    <div className="space-y-2">
                        {(preview.warnings || []).length > 0 && (
                            <ul className="notice space-y-1" data-testid={T.warnings}>
                                {preview.warnings.map((w) => (
                                    <li key={w}>{w}</li>
                                ))}
                            </ul>
                        )}
                        <MacTerm title={previewCurrent ? "What will be created" : "Out of date — preview again"} bodyClassName="!p-0">
                            <pre tabIndex={0} role="region" aria-label="Generated YAML" className="macos-term-body mono max-h-72 overflow-auto px-4 py-3 text-xs leading-[1.55]" data-testid={T.previewYaml}>{preview.yaml}</pre>
                        </MacTerm>
                    </div>
                )}

                </div>

                <DialogFooter className="gap-2">
                    <button type="button" className="btn btn-line" data-testid={T.preview} onClick={doPreview} disabled={!!busy || !complete}>
                        <Eye size={14} /> {busy === "preview" ? "Rendering…" : previewCurrent ? "Preview again" : "Preview YAML"}
                    </button>
                    <button type="button" className="btn btn-fill" data-testid={T.create} onClick={doCreate} disabled={!!busy || !previewCurrent}>
                        <Plus size={14} /> {busy === "create" ? "Creating…" : "Create"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
