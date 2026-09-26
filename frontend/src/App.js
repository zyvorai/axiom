import { lazy } from "react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeProvider } from "@/lib/theme";
import { ClusterProvider } from "@/lib/cluster";
import { AuthProvider, useAuth } from "@/lib/auth";
import AppShell from "@/components/AppShell";
import Login from "@/pages/Login";
const Overview = lazy(() => import("@/pages/Overview"));
const Onboarding = lazy(() => import("@/pages/Onboarding"));
const Nodes = lazy(() => import("@/pages/Nodes"));
const NodeDetail = lazy(() => import("@/pages/NodeDetail"));
const Workloads = lazy(() => import("@/pages/Workloads"));
const Pods = lazy(() => import("@/pages/Pods"));
const Networking = lazy(() => import("@/pages/Networking"));
const VirtualMachines = lazy(() => import("@/pages/VirtualMachines"));
const VMDetail = lazy(() => import("@/pages/VMDetail"));
const Storage = lazy(() => import("@/pages/Storage"));
const Atlas = lazy(() => import("@/pages/Atlas"));
const Namespaces = lazy(() => import("@/pages/Namespaces"));
const NamespaceDetail = lazy(() => import("@/pages/NamespaceDetail"));
const Services = lazy(() => import("@/pages/Services"));
const ServiceDetail = lazy(() => import("@/pages/ServiceDetail"));
const Ingresses = lazy(() => import("@/pages/Ingresses"));
const ConfigMaps = lazy(() => import("@/pages/ConfigMaps"));
const ConfigMapDetail = lazy(() => import("@/pages/ConfigMapDetail"));
const Quotas = lazy(() => import("@/pages/Quotas"));
const HPA = lazy(() => import("@/pages/HPA"));
const CustomResources = lazy(() => import("@/pages/CustomResources"));
const PluginView = lazy(() => import("@/pages/PluginView"));
const PlatformIntents = lazy(() => import("@/pages/PlatformIntents"));
const WorkloadDetail = lazy(() => import("@/pages/WorkloadDetail"));
const PodDetail = lazy(() => import("@/pages/PodDetail"));
const Logs = lazy(() => import("@/pages/Logs"));
const Events = lazy(() => import("@/pages/Events"));
const Jobs = lazy(() => import("@/pages/Jobs"));
const JobDetail = lazy(() => import("@/pages/JobDetail"));
const CronJobDetail = lazy(() => import("@/pages/CronJobDetail"));
const Secrets = lazy(() => import("@/pages/Secrets"));
const SecretDetail = lazy(() => import("@/pages/SecretDetail"));
const Backups = lazy(() => import("@/pages/Backups"));
const Alerts = lazy(() => import("@/pages/Alerts"));
const Ops = lazy(() => import("@/pages/Ops"));
const Settings = lazy(() => import("@/pages/Settings"));
const Users = lazy(() => import("@/pages/Users"));
const NotFound = lazy(() => import("@/pages/NotFound"));
import "@/styles/accessibility.css"; // keep last: forced-colours / reduced-motion overrides must win the cascade

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
                                <Route path="/getting-started" element={<Onboarding />} />
                                <Route path="/nodes" element={<Nodes />} />
                                <Route path="/nodes/:name" element={<NodeDetail />} />
                                <Route path="/workloads" element={<Workloads />} />
                                <Route path="/workloads/:namespace/:kind/:name" element={<WorkloadDetail />} />
                                <Route path="/pods" element={<Pods />} />
                                <Route path="/pods/:namespace/:name" element={<PodDetail />} />
                                <Route path="/platform-intents" element={<PlatformIntents />} />
                                <Route path="/networking" element={<Networking />} />
                                <Route path="/services" element={<Services />} />
                                <Route path="/services/:namespace/:name" element={<ServiceDetail />} />
                                <Route path="/ingresses" element={<Ingresses />} />
                                <Route path="/configmaps" element={<ConfigMaps />} />
                                <Route path="/configmaps/:namespace/:name" element={<ConfigMapDetail />} />
                                <Route path="/quotas" element={<Quotas />} />
                                <Route path="/autoscaling" element={<HPA />} />
                                <Route path="/custom-resources" element={<CustomResources />} />
                                <Route path="/plugins/:pluginId" element={<PluginView />} />
                                <Route path="/namespaces" element={<Namespaces />} />
                                <Route path="/namespaces/:name" element={<NamespaceDetail />} />
                                <Route path="/vms" element={<VirtualMachines />} />
                                <Route path="/vms/:namespace/:name" element={<VMDetail />} />
                                <Route path="/storage" element={<Storage />} />
                                <Route path="/atlas" element={<Atlas />} />
                                <Route path="/logs" element={<Logs />} />
                                <Route path="/events" element={<Events />} />
                                <Route path="/jobs" element={<Jobs />} />
                                <Route path="/jobs/:namespace/:name" element={<JobDetail />} />
                                <Route path="/cronjobs/:namespace/:name" element={<CronJobDetail />} />
                                <Route path="/secrets" element={<Secrets />} />
                                <Route path="/secrets/:namespace/:name" element={<SecretDetail />} />
                                <Route path="/backups" element={<Backups />} />
                                <Route path="/alerts" element={<Alerts />} />
                                <Route path="/ops" element={<Ops />} />
                                <Route path="/ops/:tab" element={<Ops />} />
                                <Route path="/settings" element={<Settings />} />
                                <Route path="/users" element={<Users />} />
                                <Route path="*" element={<NotFound />} />
                            </Route>
                        </Routes>
                    </BrowserRouter>
                    <Toaster richColors position="top-right" />
                </ClusterProvider>
            </AuthProvider>
        </ThemeProvider>
    );
}
