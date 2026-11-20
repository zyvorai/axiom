import { createContext, useContext, useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

const ClusterContext = createContext({
    clusters: [],
    active: null,
    activate: () => {},
    loading: true,
    error: null,
    refresh: () => {},
});

// Backed by GET /api/v1/clusters and POST /api/v1/clusters/{name}/activate.
// A single-cluster deployment (the common case — one kubeconfig / in-cluster
// ServiceAccount) still returns exactly one real entry here; this is not a
// cosmetic multi-region breakdown of one cluster.
export function ClusterProvider({ children }) {
    const { user } = useAuth();
    const [clusters, setClusters] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [activating, setActivating] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const { data } = await api.get("/clusters");
            setClusters(data || []);
            setError(null);
        } catch (e) {
            setError(e.message || "Failed to load clusters");
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        if (user) load();
    }, [user, load]);

    const active = clusters.find((c) => c.current) || clusters[0] || null;

    const activate = async (name) => {
        if (activating || !name || name === active?.name) return;
        setActivating(true);
        try {
            await api.post(`/clusters/${encodeURIComponent(name)}/activate`);
            window.location.reload();
        } finally {
            setActivating(false);
        }
    };

    return (
        <ClusterContext.Provider value={{ clusters, active, activate, loading, error, refresh: load }}>
            {children}
        </ClusterContext.Provider>
    );
}

export const useCluster = () => useContext(ClusterContext);
