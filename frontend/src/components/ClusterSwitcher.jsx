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
            className="block text-left transition-colors duration-200 ease-apple"
            title="Open clusters in Settings"
        >
            <div className="text-[11px] leading-[1.36] tracking-[-0.008em] text-muted-foreground">Cluster</div>
            <div className="mt-0.5 truncate text-[12px] font-medium leading-[1.33] tracking-[-0.01em] text-foreground hover:text-tone-link">
                {active.label || active.name}
            </div>
        </Link>
    );
}
