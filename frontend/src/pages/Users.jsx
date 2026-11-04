import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { UserPlus, Trash2, ShieldCheck, Eye, Pencil, KeyRound, LogOut } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { formatDate } from "@/lib/i18n";
import { useConfirm } from "@/components/ConfirmDialog";
import { MFA, LDAP_MFA } from "@/constants/testIds";

function parseNamespaces(text) {
    return text
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
}

function CreateUserDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [role, setRole] = useState("viewer");
    const [namespacesText, setNamespacesText] = useState("");
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/users", {
                username,
                password,
                role,
                namespaces: role === "viewer" || role === "operator" ? parseNamespaces(namespacesText) : [],
            });
            toast.success(t("toasts:users.created", { name: username }));
            onOpenChange(false);
            setUsername("");
            setPassword("");
            setRole("viewer");
            setNamespacesText("");
            onCreated();
        } catch (e) {
            toast.error(t("toasts:users.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("auth:users.addUser")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="users-username" className="text-xs text-muted-foreground">{t("auth:login.username")}</Label>
                        <Input id="users-username" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="jane" className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="users-password" className="text-xs text-muted-foreground">{t("auth:users.passwordMin")}</Label>
                        <Input id="users-password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="users-role" className="text-xs text-muted-foreground">{t("common:fields.role")}</Label>
                        <Select value={role} onValueChange={setRole}>
                            <SelectTrigger id="users-role" className="bg-muted/30">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="viewer">{t("auth:users.roles.viewer")}</SelectItem>
                                <SelectItem value="operator">{t("auth:users.roles.operator")}</SelectItem>
                                <SelectItem value="admin">{t("auth:users.roles.admin")}</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    {(role === "viewer" || role === "operator") && (
                        <div className="space-y-1.5">
                            <Label htmlFor="users-restrict-to-namespaces" className="text-xs text-muted-foreground">{t("auth:users.restrict")}</Label>
                            <Input id="users-restrict-to-namespaces"
                                value={namespacesText}
                                onChange={(e) => setNamespacesText(e.target.value)}
                                placeholder="team-a, team-b"
                                className="bg-muted/30"
                            />
                        </div>
                    )}
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !username || password.length < 8}>
                        {busy ? t("auth:users.creating") : t("common:actions.create")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function EditUserDialog({ user, onOpenChange, onUpdated }) {
    const { t } = useTranslation();
    const [role, setRole] = useState(user?.role || "viewer");
    const [password, setPassword] = useState("");
    const [namespacesText, setNamespacesText] = useState("");
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        setRole(user?.role || "viewer");
        setPassword("");
        setNamespacesText((user?.namespaces || []).join(", "));
    }, [user]);

    const submit = async () => {
        const patch = {};
        if (role !== user.role) patch.role = role;
        if (password) patch.password = password;
        if (role === "viewer" || role === "operator") {
            const next = parseNamespaces(namespacesText);
            const current = user.namespaces || [];
            if (next.join(",") !== current.join(",")) patch.namespaces = next;
        }
        if (Object.keys(patch).length === 0) {
            onOpenChange(false);
            return;
        }
        setBusy(true);
        try {
            await api.patch(`/users/${encodeURIComponent(user.username)}`, patch);
            toast.success(t("toasts:users.updated", { name: user.username }));
            onOpenChange(false);
            onUpdated();
        } catch (e) {
            toast.error(t("toasts:users.updateFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={!!user} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("auth:users.editTitle", { name: user?.username })}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="users-role-2" className="text-xs text-muted-foreground">{t("common:fields.role")}</Label>
                        <Select value={role} onValueChange={setRole}>
                            <SelectTrigger id="users-role-2" className="bg-muted/30">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="viewer">{t("auth:users.roles.viewer")}</SelectItem>
                                <SelectItem value="operator">{t("auth:users.roles.operator")}</SelectItem>
                                <SelectItem value="admin">{t("auth:users.roles.admin")}</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    {(role === "viewer" || role === "operator") && (
                        <div className="space-y-1.5">
                            <Label htmlFor="users-restrict-to-namespaces-2" className="text-xs text-muted-foreground">{t("auth:users.restrict")}</Label>
                            <Input id="users-restrict-to-namespaces-2"
                                value={namespacesText}
                                onChange={(e) => setNamespacesText(e.target.value)}
                                placeholder="team-a, team-b"
                                className="bg-muted/30"
                            />
                        </div>
                    )}
                    <div className="space-y-1.5">
                        <Label htmlFor="users-new-password" className="text-xs text-muted-foreground">{t("auth:users.newPassword")}</Label>
                        <Input id="users-new-password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t("auth:users.newPasswordPlaceholder")} className="bg-muted/30" />
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || (password.length > 0 && password.length < 8)}>
                        {busy ? t("common:actions.saving") : t("common:actions.save")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Users() {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [createOpen, setCreateOpen] = useState(false);
    const [editingUser, setEditingUser] = useState(null);
    const [external, setExternal] = useState([]);

    const loadExternal = () =>
        api
            .get("/users/external-mfa")
            .then((r) => setExternal(r.data || []))
            .catch(() => setExternal([]));
    const load = () => {
        loadExternal();
        return api.get("/users").then((r) => setData(r.data));
    };

    useEffect(() => {
        load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const remove = async (u) => {
        const ok = await confirm({
            title: t("dialogs:users.deleteTitle", { name: u.username }),
            description: t("dialogs:users.deleteDescription"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/users/${encodeURIComponent(u.username)}`);
            toast.success(t("toasts:users.deleted", { name: u.username }));
            load();
        } catch (e) {
            toast.error(t("toasts:users.deleteFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const resetMfa = async (u) => {
        const ok = await confirm({
            title: t("dialogs:users.resetMfaTitle", { name: u.username }),
            description: t("dialogs:users.resetMfaDescription"),
            confirmLabel: t("common:actions.reset"),
        });
        if (!ok) return;
        try {
            await api.delete(`/users/${encodeURIComponent(u.username)}/mfa`);
            toast.success(t("toasts:users.mfaReset", { name: u.username }));
            load();
        } catch (e) {
            toast.error(t("toasts:users.resetFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const resetExternalMfa = async (u) => {
        const ok = await confirm({
            title: t("dialogs:users.resetMfaTitle", { name: u.username }),
            description: t("dialogs:users.resetExternalMfaDescription"),
            confirmLabel: t("common:actions.reset"),
        });
        if (!ok) return;
        try {
            await api.delete(`/users/${encodeURIComponent(u.username)}/mfa`);
            toast.success(t("toasts:users.mfaReset", { name: u.username }));
            loadExternal();
        } catch (e) {
            toast.error(t("toasts:users.resetFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const revokeSessions = async (u) => {
        const ok = await confirm({
            title: t("dialogs:users.revokeTitle", { name: u.username }),
            description: t("dialogs:users.revokeDescription"),
            confirmLabel: t("nav:userMenu.signOut"),
        });
        if (!ok) return;
        try {
            await api.post(`/users/${encodeURIComponent(u.username)}/revoke-sessions`);
            toast.success(t("toasts:users.revoked", { name: u.username }));
        } catch (e) {
            toast.error(t("toasts:users.failed"), { description: e?.response?.data?.error || e.message });
        }
    };

    if (!data) {
        return (
            <div className="page">
                <PageHeader title={t("pages:users.title")} />
                <div className="flex flex-col gap-3.5">
                    <Skeleton className="h-16 rounded-2xl bg-muted" />
                    <Skeleton className="h-64 rounded-2xl bg-muted" />
                </div>
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title={t("pages:users.title")}
                description={t("pages:users.description")}
                action={
                    <button type="button" className="btn btn-fill btn-sm" onClick={() => setCreateOpen(true)}>
                        <UserPlus size={14} /> {t("auth:users.addUser")}
                    </button>
                }
            />

            <div className="card p-0">
                {data.length === 0 ? (
                    <div className="empty">{t("auth:users.empty")}</div>
                ) : (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("auth:login.username")}</th>
                                <th>{t("common:fields.role")}</th>
                                <th>{t("auth:users.namespaces")}</th>
                                <th>{t("auth:users.twoFactor")}</th>
                                <th>{t("common:fields.created")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {data.map((u) => (
                                <tr key={u.username}>
                                    <td className="name">{u.username}</td>
                                    <td>
                                        <span className="pill" style={{ "--c": u.role === "admin" ? "var(--green)" : "var(--ink-3)" }}>
                                            {u.role === "admin" ? <ShieldCheck size={11} /> : <Eye size={11} />}
                                            {u.role}
                                        </span>
                                    </td>
                                    <td>
                                        {u.namespaces && u.namespaces.length > 0 ? (
                                            <div className="flex flex-wrap gap-1.5">
                                                {u.namespaces.map((ns) => (
                                                    <span key={ns} className="pill soft mono text-[11px]">
                                                        {ns}
                                                    </span>
                                                ))}
                                            </div>
                                        ) : (
                                            <span className="text-[12.5px] text-ink-3">{t("nav:topbar.allNamespaces")}</span>
                                        )}
                                    </td>
                                    <td data-testid={MFA.usersBadge(u.username)}>
                                        {u.mfaEnabled ? (
                                            <span className="pill" style={{ "--c": "var(--green)" }}>
                                                <ShieldCheck size={11} /> {t("auth:users.mfaOn")}
                                            </span>
                                        ) : (
                                            <span className="text-[12.5px] text-ink-3">{t("auth:users.mfaOff")}</span>
                                        )}
                                    </td>
                                    <td className="mono num">{u.createdAt ? formatDate(u.createdAt) : "—"}</td>
                                    <td>
                                        <div className="flex justify-end gap-1.5">
                                            {u.mfaEnabled && (
                                                <button type="button" className="btn btn-line btn-sm" data-testid={MFA.usersReset(u.username)} aria-label={t("auth:users.resetMfa", { name: u.username })} title={t("auth:users.resetMfa", { name: u.username })} onClick={() => resetMfa(u)}>
                                                    <KeyRound size={13} />
                                                </button>
                                            )}
                                            <button type="button" className="btn btn-line btn-sm" data-testid={MFA.usersRevoke(u.username)} aria-label={t("auth:users.revoke", { name: u.username })} title={t("auth:users.revoke", { name: u.username })} onClick={() => revokeSessions(u)}>
                                                <LogOut size={13} />
                                            </button>
                                            <button type="button" className="btn btn-line btn-sm" aria-label={t("auth:users.edit", { name: u.username })} title={t("auth:users.edit", { name: u.username })} onClick={() => setEditingUser(u)}>
                                                <Pencil size={13} />
                                            </button>
                                            <button type="button" className="btn btn-danger btn-sm" aria-label={t("auth:users.delete", { name: u.username })} title={t("auth:users.delete", { name: u.username })} onClick={() => remove(u)}>
                                                <Trash2 size={13} />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </div>

            {external.length > 0 && (
                <div className="card p-0" data-testid={LDAP_MFA.section}>
                    <div className="panel-h">{t("auth:users.directoryTitle")}</div>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("auth:login.username")}</th>
                                <th>{t("auth:users.twoFactor")}</th>
                                <th>{t("common:fields.updated")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {external.map((u) => (
                                <tr key={u.username} data-testid={LDAP_MFA.row(u.username)}>
                                    <td className="name">{u.username}<small>{t("auth:users.directory")}</small></td>
                                    <td>
                                        {u.enabled ? (
                                            <span className="pill" style={{ "--c": "var(--green)" }}><ShieldCheck size={11} /> {t("auth:users.mfaOn")}</span>
                                        ) : (
                                            <span className="text-[12.5px] text-ink-3">{t("auth:users.mfaIncomplete")}</span>
                                        )}
                                    </td>
                                    <td className="mono num">{u.updatedAt ? formatDate(u.updatedAt) : "—"}</td>
                                    <td>
                                        <div className="flex justify-end">
                                            <button type="button" className="btn btn-line btn-sm" data-testid={LDAP_MFA.reset(u.username)} aria-label={t("auth:users.resetMfa", { name: u.username })} title={t("auth:users.resetMfa", { name: u.username })} onClick={() => resetExternalMfa(u)}>
                                                <KeyRound size={13} />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}

            <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
            <EditUserDialog user={editingUser} onOpenChange={(open) => !open && setEditingUser(null)} onUpdated={load} />
        </div>
    );
}
