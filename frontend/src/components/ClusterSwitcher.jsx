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
            <div className="rail-apple-meta text-muted-foreground">Cluster</div>
            <div className="rail-apple-cluster mt-0.5 truncate text-foreground hover:text-tone-link">
                {active.label || active.name}
            </div>
        </Link>
    );
}
