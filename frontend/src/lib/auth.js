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
                // {required, enrolled}: set when AXIOM_REQUIRE_MFA covers this account.
                mfa: data.mfa || null,
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

    // Returns "ok", "mfa" (the password was right but this account needs a
    // second factor: ask for a code and call again with it) or "denied".
    const signInLocal = useCallback(
        async (username, password, code) => {
            if (code) {
                // Two-factor accounts cannot use HTTP Basic: the server issues a
                // signed session cookie instead, so no password is kept in the tab.
                clearBasicAuth();
                try {
                    await api.post("/auth/login", { username, password, code });
                } catch (e) {
                    return e?.response?.data?.mfa ? "mfa-denied" : "denied";
                }
                return (await loadSession()) ? "ok" : "denied";
            }
            const token = btoa(`${username}:${password}`);
            setBasicAuth(token);
            try {
                await api.get("/session");
            } catch (e) {
                clearBasicAuth();
                const needsCode = e?.response?.status === 401 && e?.response?.headers?.["x-axiom-mfa"] === "required";
                return needsCode ? "mfa" : "denied";
            }
            const ok = await loadSession();
            if (!ok) clearBasicAuth();
            return ok ? "ok" : "denied";
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

// Display names for roles. The values themselves (`viewer`/`operator`/`admin`
// from /users, `Viewer`/`Platform Admin` from /session) are what the API sends
// and stores, and stay untouched; only the label shown is translated. An
// unknown value is shown as sent.
const ACCOUNT_ROLE_KEYS = { viewer: "auth:roleNames.viewer", operator: "auth:roleNames.operator", admin: "auth:roleNames.admin" };
const SESSION_ROLE_KEYS = {
    Viewer: "auth:sessionRole.viewer",
    Operator: "auth:sessionRole.operator",
    "Platform Admin": "auth:sessionRole.platformAdmin",
};

/** Label for a stored account role (`viewer`, `operator`, `admin`). */
export function accountRoleLabel(t, role) {
    return ACCOUNT_ROLE_KEYS[role] ? t(ACCOUNT_ROLE_KEYS[role]) : role;
}

/** Label for the role GET /session reports for the signed-in user. */
export function sessionRoleLabel(t, role) {
    return SESSION_ROLE_KEYS[role] ? t(SESSION_ROLE_KEYS[role]) : role;
}
