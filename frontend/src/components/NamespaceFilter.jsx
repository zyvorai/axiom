import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

// A page's namespace filter. `namespaces` is the list the page knows about (from the global
// provider and/or its own rows); the current value is always offered so a namespace from a
// shared URL that no longer exists still renders instead of an empty trigger.
export default function NamespaceFilter({ value, onChange, namespaces = [], testId }) {
    const options = [...new Set([...namespaces, ...(value && value !== "all" ? [value] : [])])].sort();
    return (
        <Select value={value} onValueChange={onChange}>
            <SelectTrigger aria-label="Filter by namespace" data-testid={testId} className="w-[180px] bg-muted/30">
                <SelectValue placeholder="Namespace" />
            </SelectTrigger>
            <SelectContent>
                <SelectItem value="all">All namespaces</SelectItem>
                {options.map((n) => (
                    <SelectItem key={n} value={n}>
                        {n}
                    </SelectItem>
                ))}
            </SelectContent>
        </Select>
    );
}
