import { createContext, useContext, useState, useCallback, useEffect } from "react";
import { api, setBasicAuth, clearBasicAuth, restoreBasicAuth } from "@/lib/api";

const AuthContext = createContext({
    user: null,
    ready: false,
    providers: null,
    signInLocal: async () => {},
    signOut: () => {},
    oidcLoginUrl: null,
    samlLoginUrl: null,
});

function initials(name) {
    return (name || "?")
        .split(/[\s.@_-]+/)
        .filter(Boolean)
        .slice(0, 2)
        .map((p) => p[0]?.toUpperCase())
        .join("") || "?";
}

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [providers, setProviders] = useState(null);
    const [ready, setReady] = useState(false);

    const loadSession = useCallback(async () => {
        try {
            const { data } = await api.get("/session");
            setUser({
                name: data.user,
                initials: initials(data.user),
                role: data.role,
                authMethod: data.auth,
                mode: data.mode,
            });
            return true;
        } catch {
            setUser(null);
            return false;
        }
    }, []);

    useEffect(() => {
        (async () => {
            try {
                const { data } = await api.get("/auth/providers");
                setProviders(data);
            } catch {
                setProviders(null);
            }
            restoreBasicAuth();
            await loadSession();
            setReady(true);
        })();
    }, [loadSession]);

    const signInLocal = useCallback(
        async (username, password) => {
            const token = btoa(`${username}:${password}`);
            setBasicAuth(token);
            const ok = await loadSession();
            if (!ok) clearBasicAuth();
            return ok;
        },
        [loadSession]
    );

    const signOut = useCallback(async () => {
        clearBasicAuth();
        try {
            await api.post("/auth/logout");
        } catch {
            // best effort — local credentials are already cleared client-side
        }
        setUser(null);
    }, []);

    const value = {
        user,
        ready,
        isViewer: user?.role === "Viewer",
        providers,
        signInLocal,
        signOut,
        refresh: loadSession,
        oidcLoginUrl: providers?.oidc?.enabled ? providers.oidc.login_url : null,
        samlLoginUrl: providers?.saml?.enabled ? providers.saml.login_url : null,
        labHint: providers?.lab?.operator_login ? providers.lab.hint : null,
        defaultUsername: providers?.local?.default_username || "",
    };

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export const useAuth = () => useContext(AuthContext);
