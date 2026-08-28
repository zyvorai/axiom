import { createContext, useContext, useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

const ClusterContext = createContext({
    clusters: [],
    activeId: null,
    setActiveId: () => {},
    active: null,
    loading: true,
    error: null,
    session: null,
    refresh: () => {},
});

// Axiom connects to a single Kubernetes cluster per instance. The "regions"
// returned on the session are a cosmetic multi-region breakdown of that one
// cluster (see internal/platform.DefaultRegions on the backend) — switching
// the active entry here does not change which cluster API calls hit.
export function ClusterProvider({ children }) {
    const { user } = useAuth();
    const [session, setSession] = useState(null);
    const [activeId, setActiveIdState] = useState(() => localStorage.getItem("axiom.activeRegion") || null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const { data } = await api.get("/session");
            setSession(data);
            setError(null);
        } catch (e) {
            setError(e.message || "Failed to load session");
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        if (user) load();
    }, [user, load]);

    const regions = session?.regions || [];
    const clusters = regions.map((r) => ({
        id: r.id,
        name: r.label,
        status: "Healthy",
        provider: session?.cluster || "Kubernetes",
        region: r.id,
    }));

    useEffect(() => {
        if (!clusters.length) return;
        const still = clusters.find((c) => c.id === activeId);
        if (still) return;
        const def = regions.find((r) => r.active) || regions[0];
        if (def) setActiveIdState(def.id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [session]);

    const setActiveId = (id) => {
        setActiveIdState(id);
        localStorage.setItem("axiom.activeRegion", id);
    };

    const active = clusters.find((c) => c.id === activeId) || clusters[0] || null;

    return (
        <ClusterContext.Provider value={{ clusters, activeId, setActiveId, active, loading, error, session, refresh: load }}>
            {children}
        </ClusterContext.Provider>
    );
}

export const useCluster = () => useContext(ClusterContext);
