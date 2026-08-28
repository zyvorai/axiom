import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import "@/App.css";
import { ThemeProvider } from "@/lib/theme";
import { ClusterProvider } from "@/lib/cluster";
import { AuthProvider, useAuth } from "@/lib/auth";
import AppShell from "@/components/AppShell";
import Login from "@/pages/Login";
import Overview from "@/pages/Overview";
import Nodes from "@/pages/Nodes";
import Workloads from "@/pages/Workloads";
import Pods from "@/pages/Pods";
import Networking from "@/pages/Networking";
import VirtualMachines from "@/pages/VirtualMachines";
import Storage from "@/pages/Storage";
import Atlas from "@/pages/Atlas";
import Namespaces from "@/pages/Namespaces";
import Services from "@/pages/Services";
import Logs from "@/pages/Logs";
import Events from "@/pages/Events";
import Secrets from "@/pages/Secrets";
import Backups from "@/pages/Backups";
import Alerts from "@/pages/Alerts";
import Ops from "@/pages/Ops";
import Settings from "@/pages/Settings";

function Protected({ children }) {
    const { user, ready } = useAuth();
    if (!ready) return null;
    if (!user) return <Navigate to="/login" replace />;
    return children;
}

function LoginGate() {
    const { user, ready } = useAuth();
    if (!ready) return null;
    if (user) return <Navigate to="/overview" replace />;
    return <Login />;
}

export default function App() {
    return (
        <ThemeProvider>
            <AuthProvider>
                <ClusterProvider>
                    <BrowserRouter>
                        <Routes>
                            <Route path="/login" element={<LoginGate />} />
                            <Route
                                element={
                                    <Protected>
                                        <AppShell />
                                    </Protected>
                                }
                            >
                                <Route path="/" element={<Navigate to="/overview" replace />} />
                                <Route path="/overview" element={<Overview />} />
                                <Route path="/nodes" element={<Nodes />} />
                                <Route path="/workloads" element={<Workloads />} />
                                <Route path="/pods" element={<Pods />} />
                                <Route path="/networking" element={<Networking />} />
                                <Route path="/services" element={<Services />} />
                                <Route path="/namespaces" element={<Namespaces />} />
                                <Route path="/vms" element={<VirtualMachines />} />
                                <Route path="/storage" element={<Storage />} />
                                <Route path="/atlas" element={<Atlas />} />
                                <Route path="/logs" element={<Logs />} />
                                <Route path="/events" element={<Events />} />
                                <Route path="/secrets" element={<Secrets />} />
                                <Route path="/backups" element={<Backups />} />
                                <Route path="/alerts" element={<Alerts />} />
                                <Route path="/ops" element={<Ops />} />
                                <Route path="/ops/:tab" element={<Ops />} />
                                <Route path="/settings" element={<Settings />} />
                                <Route path="*" element={<Navigate to="/overview" replace />} />
                            </Route>
                        </Routes>
                    </BrowserRouter>
                    <Toaster richColors position="top-right" />
                </ClusterProvider>
            </AuthProvider>
        </ThemeProvider>
    );
}
