/** Best-effort deep link from Kubernetes involvedObject-style "Kind/name". */
export function resourceHref(namespace, object) {
    if (!object) return null;
    const raw = String(object);
    const slash = raw.indexOf("/");
    const kind = (slash >= 0 ? raw.slice(0, slash) : "").toLowerCase();
    const name = slash >= 0 ? raw.slice(slash + 1) : raw;
    if (!name) return null;
    const ns = namespace ? encodeURIComponent(namespace) : "";
    const n = encodeURIComponent(name);
    switch (kind) {
        case "pod":
            return ns ? `/pods/${ns}/${n}` : null;
        case "deployment":
        case "statefulset":
        case "daemonset":
        case "replicaset":
            return ns ? `/workloads/${ns}/${kind}/${n}` : null;
        case "job":
            return ns ? `/jobs/${ns}/${n}` : null;
        case "cronjob":
            return ns ? `/cronjobs/${ns}/${n}` : null;
        case "service":
            return ns ? `/services/${ns}/${n}` : null;
        case "configmap":
            return ns ? `/configmaps/${ns}/${n}` : null;
        case "secret":
            return ns ? `/secrets/${ns}/${n}` : null;
        case "virtualmachine":
        case "virtualmachineinstance":
            return ns ? `/vms/${ns}/${n}` : null;
        case "node":
            return `/nodes/${n}`;
        case "namespace":
            return `/namespaces/${n}`;
        default:
            if (!kind && ns) return `/namespaces/${ns}`;
            return null;
    }
}
