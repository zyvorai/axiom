import { cn } from "@/lib/utils";

// Netra page-hero — see .head in axiom-shell.css: optional blue eyebrow,
// tight 600-weight h1, lede, over a soft radial glow. `tools` is an optional
// row of filters/search rendered beneath the title.
export default function PageHeader({ eyebrow, title, description, action, tools, className }) {
    return (
        <header className={cn("head", className)}>
            <div className="head-row">
                <div className="min-w-0">
                    {eyebrow && <div className="eyebrow">{eyebrow}</div>}
                    <h1>{title}</h1>
                    {description && <p>{description}</p>}
                </div>
                {action && <div className="r">{action}</div>}
            </div>
            {tools && <div className="tools">{tools}</div>}
        </header>
    );
}
