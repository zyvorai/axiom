import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { UserPlus, Trash2, ShieldCheck, Eye, Pencil } from "lucide-react";
import { toast } from "sonner";

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
                namespaces: role === "viewer" ? parseNamespaces(namespacesText) : [],
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
            <DialogContent className="glass max-w-md border-white/10">
                <DialogHeader>
                    <DialogTitle>Add user</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Username</Label>
                        <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="jane" className="border-white/10 bg-white/[0.03]" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Password (min 8 characters)</Label>
                        <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="border-white/10 bg-white/[0.03]" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Role</Label>
                        <Select value={role} onValueChange={setRole}>
                            <SelectTrigger className="border-white/10 bg-white/[0.03]">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="viewer">Viewer — read only</SelectItem>
                                <SelectItem value="admin">Admin — full access</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    {role === "viewer" && (
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Restrict to namespaces (comma-separated, blank = all)</Label>
                            <Input
                                value={namespacesText}
                                onChange={(e) => setNamespacesText(e.target.value)}
                                placeholder="team-a, team-b"
                                className="border-white/10 bg-white/[0.03]"
                            />
                        </div>
                    )}
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !username || password.length < 8} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        {busy ? "Creating…" : "Create"}
                    </Button>
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
        if (role === "viewer") {
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
            <DialogContent className="glass max-w-md border-white/10">
                <DialogHeader>
                    <DialogTitle>Edit {user?.username}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Role</Label>
                        <Select value={role} onValueChange={setRole}>
                            <SelectTrigger className="border-white/10 bg-white/[0.03]">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="viewer">Viewer — read only</SelectItem>
                                <SelectItem value="admin">Admin — full access</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    {role === "viewer" && (
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Restrict to namespaces (comma-separated, blank = all)</Label>
                            <Input
                                value={namespacesText}
                                onChange={(e) => setNamespacesText(e.target.value)}
                                placeholder="team-a, team-b"
                                className="border-white/10 bg-white/[0.03]"
                            />
                        </div>
                    )}
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">New password (leave blank to keep current)</Label>
                        <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="min 8 characters" className="border-white/10 bg-white/[0.03]" />
                    </div>
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || (password.length > 0 && password.length < 8)} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        {busy ? "Saving…" : "Save"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Users() {
    const [data, setData] = useState(null);
    const [createOpen, setCreateOpen] = useState(false);
    const [editingUser, setEditingUser] = useState(null);

    const load = () => api.get("/users").then((r) => setData(r.data));

    useEffect(() => {
        load();
    }, []);

    const remove = async (u) => {
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
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Users</h1>
                    <p className="text-sm text-muted-foreground">
                        Local accounts alongside SSO. Viewers can read every page but cannot create, change, or delete anything.
                    </p>
                </div>
                <Button size="sm" onClick={() => setCreateOpen(true)} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                    <UserPlus className="mr-1.5 h-3.5 w-3.5" /> Add user
                </Button>
            </div>

            <GlassCard className="p-0 overflow-hidden">
                <Table>
                    <TableHeader>
                        <TableRow className="border-white/5 hover:bg-transparent">
                            <TableHead>Username</TableHead>
                            <TableHead>Role</TableHead>
                            <TableHead>Namespaces</TableHead>
                            <TableHead>Created</TableHead>
                            <TableHead />
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {data.map((u) => (
                            <TableRow key={u.username} className="border-white/5 hover:bg-white/[0.03]">
                                <TableCell className="font-medium">{u.username}</TableCell>
                                <TableCell>
                                    <Badge
                                        variant="outline"
                                        className={
                                            u.role === "admin"
                                                ? "border-[#93c88f]/40 bg-[#93c88f]/10 text-[#93c88f]"
                                                : "border-white/10 bg-white/[0.03] text-muted-foreground"
                                        }
                                    >
                                        {u.role === "admin" ? (
                                            <ShieldCheck className="mr-1 h-3 w-3" />
                                        ) : (
                                            <Eye className="mr-1 h-3 w-3" />
                                        )}
                                        {u.role}
                                    </Badge>
                                </TableCell>
                                <TableCell>
                                    {u.namespaces && u.namespaces.length > 0 ? (
                                        <div className="flex flex-wrap gap-1">
                                            {u.namespaces.map((ns) => (
                                                <Badge key={ns} variant="outline" className="mono text-[10px] border-white/10 bg-white/[0.03]">
                                                    {ns}
                                                </Badge>
                                            ))}
                                        </div>
                                    ) : (
                                        <span className="text-xs text-muted-foreground">All namespaces</span>
                                    )}
                                </TableCell>
                                <TableCell className="mono text-xs text-muted-foreground">
                                    {u.createdAt ? new Date(u.createdAt).toLocaleString() : "—"}
                                </TableCell>
                                <TableCell>
                                    <div className="flex justify-end gap-1">
                                        <Button size="sm" variant="ghost" onClick={() => setEditingUser(u)} className="text-muted-foreground hover:text-[#6ab6f0]">
                                            <Pencil className="h-3.5 w-3.5" />
                                        </Button>
                                        <Button size="sm" variant="ghost" onClick={() => remove(u)} className="text-muted-foreground hover:text-rose-400">
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </Button>
                                    </div>
                                </TableCell>
                            </TableRow>
                        ))}
                        {data.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                                    No local accounts yet — the env-configured operator account signs in separately.
                                </TableCell>
                            </TableRow>
                        )}
                    </TableBody>
                </Table>
            </GlassCard>

            <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
            <EditUserDialog user={editingUser} onOpenChange={(open) => !open && setEditingUser(null)} onUpdated={load} />
        </div>
    );
}
