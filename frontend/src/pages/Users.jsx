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
import { UserPlus, Trash2, ShieldCheck, Eye, Pencil } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";

function parseNamespaces(text) {
    return text
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
}

function CreateUserDialog({ open, onOpenChange, onCreated }) {
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
            toast.success(`User ${username} created`);
            onOpenChange(false);
            setUsername("");
            setPassword("");
            setRole("viewer");
            setNamespacesText("");
            onCreated();
        } catch (e) {
            toast.error("Create failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>Add user</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="users-username" className="text-xs text-muted-foreground">Username</Label>
                        <Input id="users-username" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="jane" className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="users-password" className="text-xs text-muted-foreground">Password (min 8 characters)</Label>
                        <Input id="users-password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="users-role" className="text-xs text-muted-foreground">Role</Label>
                        <Select value={role} onValueChange={setRole}>
                            <SelectTrigger id="users-role" className="bg-muted/30">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="viewer">Viewer — read only</SelectItem>
                                <SelectItem value="operator">Operator — day-2 writes, no user/cluster admin</SelectItem>
                                <SelectItem value="admin">Admin — full access</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    {(role === "viewer" || role === "operator") && (
                        <div className="space-y-1.5">
                            <Label htmlFor="users-restrict-to-namespaces" className="text-xs text-muted-foreground">Restrict to namespaces (comma-separated, blank = all)</Label>
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
                        {busy ? "Creating…" : "Create"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function EditUserDialog({ user, onOpenChange, onUpdated }) {
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
            toast.success(`Updated ${user.username}`);
            onOpenChange(false);
            onUpdated();
        } catch (e) {
            toast.error("Update failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={!!user} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>Edit {user?.username}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="users-role-2" className="text-xs text-muted-foreground">Role</Label>
                        <Select value={role} onValueChange={setRole}>
                            <SelectTrigger id="users-role-2" className="bg-muted/30">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="viewer">Viewer — read only</SelectItem>
                                <SelectItem value="operator">Operator — day-2 writes, no user/cluster admin</SelectItem>
                                <SelectItem value="admin">Admin — full access</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    {(role === "viewer" || role === "operator") && (
                        <div className="space-y-1.5">
                            <Label htmlFor="users-restrict-to-namespaces-2" className="text-xs text-muted-foreground">Restrict to namespaces (comma-separated, blank = all)</Label>
                            <Input id="users-restrict-to-namespaces-2"
                                value={namespacesText}
                                onChange={(e) => setNamespacesText(e.target.value)}
                                placeholder="team-a, team-b"
                                className="bg-muted/30"
                            />
                        </div>
                    )}
                    <div className="space-y-1.5">
                        <Label htmlFor="users-new-password" className="text-xs text-muted-foreground">New password (leave blank to keep current)</Label>
                        <Input id="users-new-password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="min 8 characters" className="bg-muted/30" />
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || (password.length > 0 && password.length < 8)}>
                        {busy ? "Saving…" : "Save"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Users() {
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [createOpen, setCreateOpen] = useState(false);
    const [editingUser, setEditingUser] = useState(null);

    const load = () => api.get("/users").then((r) => setData(r.data));

    useEffect(() => {
        load();
    }, []);

    const remove = async (u) => {
        const ok = await confirm({
            title: `Delete user ${u.username}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/users/${encodeURIComponent(u.username)}`);
            toast.success(`Deleted ${u.username}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (!data) {
        return (
            <div className="page">
                <PageHeader title="Users" />
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
                title="Users"
                description="Local accounts alongside SSO. Viewers can read every page but cannot create, change, or delete anything."
                action={
                    <button type="button" className="btn btn-fill btn-sm" onClick={() => setCreateOpen(true)}>
                        <UserPlus size={14} /> Add user
                    </button>
                }
            />

            <div className="card p-0">
                {data.length === 0 ? (
                    <div className="empty">No local accounts yet — the env-configured operator account signs in separately.</div>
                ) : (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Username</th>
                                <th>Role</th>
                                <th>Namespaces</th>
                                <th>Created</th>
                                <th><span className="sr-only">Actions</span></th>
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
                                            <span className="text-[12.5px] text-ink-3">All namespaces</span>
                                        )}
                                    </td>
                                    <td className="mono num">{u.createdAt ? new Date(u.createdAt).toLocaleString() : "—"}</td>
                                    <td>
                                        <div className="flex justify-end gap-1.5">
                                            <button type="button" className="btn btn-line btn-sm" aria-label={`Edit user ${u.username}`} title={`Edit user ${u.username}`} onClick={() => setEditingUser(u)}>
                                                <Pencil size={13} />
                                            </button>
                                            <button type="button" className="btn btn-danger btn-sm" aria-label={`Delete user ${u.username}`} title={`Delete user ${u.username}`} onClick={() => remove(u)}>
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

            <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
            <EditUserDialog user={editingUser} onOpenChange={(open) => !open && setEditingUser(null)} onUpdated={load} />
        </div>
    );
}
