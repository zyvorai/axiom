import { useMemo, useState } from "react";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Pencil, Eye } from "lucide-react";
import MacTerm from "@/components/MacTerm";

function tokenizeJson(src) {
    const tokens = [];
    const re =
        /("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b|([{}\[\],:])|(\s+)|([^\s"{}\[\],:]+)/g;
    let m;
    while ((m = re.exec(src))) {
        if (m[1] != null) {
            tokens.push({ t: "key", v: m[1] });
            tokens.push({ t: "punct", v: ":" });
        } else if (m[2] != null) tokens.push({ t: "str", v: m[2] });
        else if (m[3] != null) tokens.push({ t: "num", v: m[3] });
        else if (m[4] != null) tokens.push({ t: "kw", v: m[4] });
        else if (m[5] != null) tokens.push({ t: "punct", v: m[5] });
        else if (m[6] != null) tokens.push({ t: "ws", v: m[6] });
        else if (m[7] != null) tokens.push({ t: "plain", v: m[7] });
    }
    return tokens;
}

function Highlighted({ value }) {
    const tokens = useMemo(() => tokenizeJson(value || ""), [value]);
    return (
        <pre className="code-pane max-h-[65vh] min-h-[65vh] overflow-auto whitespace-pre px-4 py-3 text-[12.5px] leading-[1.55]">
            {tokens.map((tok, i) => (
                <span key={i} className={`tok-${tok.t}`}>
                    {tok.v}
                </span>
            ))}
        </pre>
    );
}

/** macOS Terminal–style colorful JSON/manifest viewer. An editable instance
 * (PodDetail/WorkloadDetail's manifest tab) opens straight into edit mode —
 * it exists to be edited, not previewed first — while a read-only instance
 * (CustomResources/PluginView/Networking's raw viewers) never shows the
 * toggle at all. */
export default function CodeEditor({ value, onChange, editable = true, className, title = "Manifest.json" }) {
    const [editing, setEditing] = useState(editable);

    return (
        <MacTerm
            title={title}
            className={className}
            bodyClassName="!p-0"
            actions={
                editable ? (
                    <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        className="h-6 gap-1 rounded-md px-1.5 text-[11px] text-term-dim hover:bg-term-hover hover:text-term-text"
                        onClick={() => setEditing((e) => !e)}
                    >
                        {editing ? (
                            <>
                                <Eye className="h-3 w-3" /> Preview
                            </>
                        ) : (
                            <>
                                <Pencil className="h-3 w-3" /> Edit
                            </>
                        )}
                    </Button>
                ) : null
            }
        >
            {editing ? (
                <Textarea
                    value={value}
                    onChange={(e) => onChange?.(e.target.value)}
                    className={cn(
                        "macos-term-body mono max-h-[65vh] min-h-[65vh] resize-none whitespace-pre rounded-none border-none px-4 py-3 text-[12.5px] leading-[1.55] focus-visible:ring-0"
                    )}
                    spellCheck={false}
                />
            ) : (
                <Highlighted value={value} />
            )}
        </MacTerm>
    );
}
