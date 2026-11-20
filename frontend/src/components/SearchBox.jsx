import { Search } from "lucide-react";

// The page search field. `data-search-input` is what the "/" shortcut (lib/shortcuts.js) focuses.
export default function SearchBox({ value, onChange, placeholder, label, testId, className = "" }) {
    return (
        <div className={`input ${className}`}>
            <Search size={15} className="text-ink-3" aria-hidden="true" />
            <input
                data-search-input
                data-testid={testId}
                aria-label={label || placeholder}
                placeholder={placeholder}
                value={value}
                onChange={(e) => onChange(e.target.value)}
                className="bare-input"
            />
        </div>
    );
}
