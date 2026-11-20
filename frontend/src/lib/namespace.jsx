import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import { useUrlState } from "@/lib/urlState";

const STORAGE_KEY = "axiom-namespace";
export const ALL_NAMESPACES = "all";

const NamespaceContext = createContext({
    namespace: ALL_NAMESPACES,
    namespaces: [],
    setNamespace: () => {},
});

function readStored() {
    try {
        return localStorage.getItem(STORAGE_KEY) || ALL_NAMESPACES;
    } catch {
        return ALL_NAMESPACES;
    }
}

/**
 * The cluster-wide namespace the whole console is scoped to (top-bar selector). Persisted in the
 * browser. Pages default their own namespace filter to it; a page can still override it through
 * its own `?ns=` URL parameter (see useNamespaceFilter), and choosing a namespace in the top bar
 * clears that override so the choice takes effect everywhere.
 */
export function NamespaceProvider({ children }) {
    const navigate = useNavigate();
    const [namespace, setNs] = useState(readStored);
    const [namespaces, setNamespaces] = useState([]);

    useEffect(() => {
        let alive = true;
        api
            .get("/namespaces")
            .then((r) => {
                if (!alive) return;
                const names = (Array.isArray(r.data) ? r.data : []).map((n) => n.name).filter(Boolean);
                setNamespaces([...new Set(names)].sort());
            })
            .catch(() => {});
        return () => {
            alive = false;
        };
    }, []);

    const setNamespace = useCallback(
        (next) => {
            const v = next || ALL_NAMESPACES;
            setNs(v);
            try {
                localStorage.setItem(STORAGE_KEY, v);
            } catch {
                /* private mode — applies for this session only */
            }
            const sp = new URLSearchParams(window.location.search);
            if (sp.has("ns")) {
                sp.delete("ns");
                const qs = sp.toString();
                navigate({ pathname: window.location.pathname, search: qs ? `?${qs}` : "" }, { replace: true });
            }
        },
        [navigate]
    );

    const value = useMemo(() => ({ namespace, namespaces, setNamespace }), [namespace, namespaces, setNamespace]);
    return <NamespaceContext.Provider value={value}>{children}</NamespaceContext.Provider>;
}

export const useNamespace = () => useContext(NamespaceContext);

/**
 * A page's namespace filter: the `?ns=` URL parameter if present, otherwise the global selection.
 * Returns [namespace, setNamespace, knownNamespaces, globalNamespace]; "all" means every namespace.
 */
export function useNamespaceFilter() {
    const { namespace: global, namespaces } = useNamespace();
    const [ns, setNs] = useUrlState("ns", global);
    return [ns, setNs, namespaces, global];
}
