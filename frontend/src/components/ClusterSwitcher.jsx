import { Link } from "react-router-dom";
import { useCluster } from "@/lib/cluster";
import { NAV } from "@/constants/testIds";

/** Plain text current-cluster link. Health + activate live on Settings → Clusters. */
export default function ClusterSwitcher() {
    const { active } = useCluster();
    if (!active) return null;

    return (
        <Link
            to="/settings#clusters"
            data-testid={NAV.clusterSwitcher}
            className="block text-left transition-colors duration-150"
            title="Open clusters in Settings"
        >
            <div className="text-[11px] font-normal tracking-[-0.01em] text-[#86868b] dark:text-[#6e7681]">Cluster</div>
            <div className="mt-0.5 truncate text-[12px] font-medium tracking-[-0.01em] text-[#1d1d1f] hover:text-[#0066cc] dark:text-[#e6edf3] dark:hover:text-[#2f81f7]">
                {active.label || active.name}
            </div>
        </Link>
    );
}
