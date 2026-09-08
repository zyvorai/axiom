import { cn } from "@/lib/utils";

// Flat apple.com page head — see .head in axiom-shell.css. No tracked-caps
// eyebrow (apple.com never uses one on an in-app page); `tools` is an
// optional row of filters/search rendered beneath the title.
export default function PageHeader({ title, description, action, tools, className }) {
    return (
        <header className={cn("head", className)}>
            <div className="head-row">
                <div className="min-w-0">
                    <h1>{title}</h1>
                    {description && <p>{description}</p>}
                </div>
                {action && <div className="r">{action}</div>}
            </div>
            {tools && <div className="tools">{tools}</div>}
        </header>
    );
}
